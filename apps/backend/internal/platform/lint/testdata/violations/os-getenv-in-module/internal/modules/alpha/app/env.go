package app

import "os"

func Region() string {
	return os.Getenv("REGION")
}
