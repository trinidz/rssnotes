package models

type FollowListAction int

const (
	Sync FollowListAction = iota
	Add
	Delete
)

type FollowManagment struct {
	Action       FollowListAction
	FollowEntity Entity
}
