package spotify

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type SpotifyAuthServer struct {
	port         int
	listener     net.Listener
	httpClient   *http.Client
	refreshToken string
	bearerToken  string
	bearerExpire time.Time

	clientID     string
	clientSecret string
}

type exchangeCodeResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type refreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func NewSpotifyAuthServer(port int, clientID, clientSecret string, refreshToken string) *SpotifyAuthServer {
	return &SpotifyAuthServer{
		port:         port,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
	}
}

func (a *SpotifyAuthServer) Start() (error, <-chan error) {
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.StartServer()
	}()
	authorizeURL, err := a.buildAuthorizationURL(a.clientID)
	if err != nil {
		return fmt.Errorf("failed to build Spotify authorization URL: %w", err), errCh
	}

	if err := a.openBrowser(authorizeURL); err != nil {
		fmt.Printf("Could not open your browser automatically. Please open this URL manually:\n%s\n", authorizeURL)
	} else {
		fmt.Println("Opened your browser for Spotify authorization.")
		fmt.Printf("If no page appears, open this URL manually:\n%s\n", authorizeURL)
	}
	return nil, errCh
}

func (a *SpotifyAuthServer) handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if errorText := query.Get("error"); errorText != "" {
		fmt.Printf("Spotify authorization was denied or failed: %s\n", errorText)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Spotify authorization failed. You can close this window and try again from the terminal."))
		return
	}

	err := a.exchangeCodeForBearer(query.Get("code"))
	if err != nil {
		fmt.Printf("Failed to exchange authorization code for tokens: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Could not complete Spotify authorization. Check terminal logs for details."))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Spotify authorization successful. You can close this window and return to the terminal."))

	go func() {
		time.Sleep(100 * time.Millisecond)
		if a.listener != nil {
			_ = a.listener.Close()
		}
	}()
}

func (a *SpotifyAuthServer) StartServer() error {
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", a.port))
	if err != nil {
		return fmt.Errorf("failed to start Spotify auth server: %w", err)
	}
	a.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleSpotifyCallback)
	fmt.Printf("Waiting for Spotify callback on %s ...\n", a.RedirectURI())
	err = http.Serve(listener, mux)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

func (a *SpotifyAuthServer) GetBearerToken() (string, error) {
	if time.Now().After(a.bearerExpire) {
		if err := a.refreshBearerToken(); err != nil {
			return "", err
		}
	}
	return a.bearerToken, nil
}

func (a *SpotifyAuthServer) refreshBearerToken() error {
	form := url.Values{}
	form.Add("grant_type", "refresh_token")
	form.Add("refresh_token", a.refreshToken)
	resBody := refreshTokenResponse{}
	err := a.requestTokenEndpoint(form, &resBody)
	if err != nil {
		slog.Error("Failed to refresh Spotify bearer token", "error", err)
		return err
	}
	if resBody.AccessToken == "" {
		slog.Error("Spotify token refresh response did not contain access token")
		return fmt.Errorf("Spotify token refresh response did not contain access token")
	}
	a.bearerToken = resBody.AccessToken
	a.bearerExpire = time.Now().Add(time.Duration(resBody.ExpiresIn) * time.Second)
	return nil
}

// buildAuthorizationURL implements step 1 of Spotify's Authorization Code Flow:
// redirect the user to the returned URL and persist/verify the returned state.
func (a *SpotifyAuthServer) buildAuthorizationURL(clientId string) (string, error) {
	q := url.Values{}
	q.Set("client_id", clientId)
	q.Set("response_type", "code")
	q.Set("redirect_uri", a.RedirectURI())

	return "https://accounts.spotify.com/authorize?" + q.Encode(), nil
}

func (a *SpotifyAuthServer) exchangeCodeForBearer(code string) error {
	form := url.Values{}
	form.Add("grant_type", "authorization_code")
	form.Add("code", code)
	form.Add("redirect_uri", a.RedirectURI())
	responseBody := &exchangeCodeResponse{}
	err := a.requestTokenEndpoint(form, responseBody)
	if err != nil {
		return fmt.Errorf("failed to request token endpoint: %w", err)
	}
	a.bearerToken = responseBody.AccessToken
	a.bearerExpire = time.Now().Add(time.Duration(responseBody.ExpiresIn) * time.Second)
	a.refreshToken = responseBody.RefreshToken
	return nil
}

func (a *SpotifyAuthServer) requestTokenEndpoint(form url.Values, resBody any) error {
	uri := "https://accounts.spotify.com/api/token"
	headers := http.Header{}
	headers.Add("Authorization", a.getBasicAuthHeader())
	headers.Add("Content-Type", "application/x-www-form-urlencoded")
	httpReq, err := http.NewRequest("POST", uri, strings.NewReader(form.Encode()))
	if err != nil {
		slog.Error("Failed to create HTTP request for Spotify auth token", "error", err)
		return err
	}
	httpReq.Header = headers
	res, err := a.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("Failed to execute HTTP request for Spotify auth token", "error", err)
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.Error("Received non-OK response from Spotify auth token endpoint", "status", res.StatusCode)
		bodyBytes, _ := io.ReadAll(res.Body)
		slog.Warn("Response body from Spotify auth token endpoint", "body", string(bodyBytes))
		return fmt.Errorf("received non-OK response from Spotify auth token endpoint: %d", res.StatusCode)
	}

	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(resBody); err != nil {
		return fmt.Errorf("failed to decode response body from Spotify auth token endpoint: %w", err)
	}
	return nil
}

func (a *SpotifyAuthServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", a.port)
}

func (a *SpotifyAuthServer) openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func (a *SpotifyAuthServer) RefreshToken() string {
	return a.refreshToken
}

func (a *SpotifyAuthServer) getBasicAuthHeader() string {
	credentials := fmt.Sprintf("%s:%s", a.clientID, a.clientSecret)
	sEnc := base64.StdEncoding.EncodeToString([]byte(credentials))
	return fmt.Sprintf("Basic %s", sEnc)
}
