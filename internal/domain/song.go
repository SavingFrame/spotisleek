package domain

import "time"

type Song struct {
	Artists  []string
	Album    string
	Title    string
	Duration time.Duration
	Exists   bool
	FilePath string
}
