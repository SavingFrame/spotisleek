# spotisleek

Keep your Navidrome library in sync with songs from selected Spotify playlists.

## Goal

`spotisleek` runs in the background on a schedule.

At each run it:

1. Reads tracks from configured Spotify playlist(s)
2. Checks whether each track already exists in Navidrome (Subsonic API)
3. Queues missing tracks in `slskd`
4. Downloads files into a music folder watched by Navidrome

## Runtime Model

This project is intended to run as a **background worker/service**, not an interactive CLI tool.

Typical deployment options:

- systemd service + timer
- Docker container with internal scheduler
- always-on process with periodic sync interval

## Planned Features

- Periodic sync scheduler
- Spotify playlist scanner
- Navidrome/Subsonic track existence check
- `slskd` integration for automated downloads
- Configurable sync interval
- Structured logs and sync summary (found / missing / queued / failed)
- Safe mode to validate behavior before enabling downloads

## Tech Stack

- Language: Go
- Integrations:
  - Spotify Web API
  - Navidrome (Subsonic-compatible API)
  - `slskd` HTTP API

## Configuration (planned)

Environment variables (or config file):

- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`
- `SPOTIFY_REDIRECT_URI`
- `SPOTIFY_REFRESH_TOKEN`
- `SPOTIFY_PLAYLISTS` (comma-separated playlist URLs/IDs)
- `NAVIDROME_URL`
- `NAVIDROME_USER`
- `NAVIDROME_PASSWORD`
- `SLSKD_URL`
- `SLSKD_API_KEY`
- `MUSIC_DOWNLOAD_DIR`
- `SYNC_INTERVAL` (e.g. `30m`, `1h`)

## Roadmap

- [ ] Define service config and scheduler
- [ ] Implement Spotify playlist fetch
- [ ] Implement Navidrome lookup
- [ ] Implement `slskd` queueing
- [ ] Add end-to-end periodic sync loop
- [ ] Add retry/backoff and rate-limit handling
- [ ] Add tests for sync decisions and failure handling

## Disclaimer

Please follow laws, platform ToS, and licensing rules in your country when acquiring music.
