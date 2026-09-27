package app

func Spawn(fn func()) {
	go fn()
}
