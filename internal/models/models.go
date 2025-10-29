package models

import "time"

type User struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
}

type Post struct {
	ID       uint64    `json:"id"`
	AuthorID uint64    `json:"author_id"`
	Body     string    `json:"body"`
	Created  time.Time `json:"created"`
}

type Follow struct {
	UserID     uint64 `json:"user_id"`
	FolloweeID uint64 `json:"followee_id"`
}
