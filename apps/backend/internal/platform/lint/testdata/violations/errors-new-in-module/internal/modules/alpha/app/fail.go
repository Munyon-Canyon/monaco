package app

import "errors"

func Fail() error {
	return errors.New("failed")
}
