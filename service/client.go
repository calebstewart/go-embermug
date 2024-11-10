package service

import "context"

type Client struct {
	Cancel  func()
	Channel chan State
	Context context.Context
	ID      string
}
