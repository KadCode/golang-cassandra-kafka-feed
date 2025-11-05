package models

import "time"

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type Post struct {
	ID       string    `json:"id"`
	AuthorID string    `json:"author_id"`
	Body     string    `json:"body"`
	Created  time.Time `json:"created"`
}

type Follow struct {
	UserID     string `json:"user_id"`
	FolloweeID string `json:"followee_id"`
}
