package domain

import "time"

type Song struct {
	Artist   string
	Title    string
	Duration time.Duration
	Exists   bool
}
