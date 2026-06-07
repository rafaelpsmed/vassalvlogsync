package notify

import (
	"github.com/gen2brain/beeep"
)

func Notify(title, message string) error {
	return beeep.Notify(title, message, "")
}
