package domain

import "time"

type Song struct {
	Artists  []string
	Title    string
	Duration time.Duration
	Exists   bool
}
