package misc

import (
	"fmt"
	"time"
)

func TimeNow() time.Time {
	loc, _ := time.LoadLocation("Europe/Berlin")
	currentTime := time.Now().In(loc)
	fmt.Println(currentTime)
	return currentTime
}
