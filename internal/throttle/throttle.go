package throttle

import "time"

var cache = map[string]User{}

const maxWatering = 2

type User struct {
	ScreenName string
	// The number of times user has watered in current cycle.
	CurrentWaters int
	// Total number of times user has watered.
	TotalWaters int
	// Last watered.
	LastWatered time.Time
}

func GetUser(screenName string) User {
	return cache[screenName]
}

func UserCanWater(screenName string) bool {
	// Check for user in the cache.
	user, hasWatered := cache[screenName]
	if !hasWatered {
		// If user is not already in cache, allow.
		return true
	}

	// If user's current watering in cycle is less than max, allow.
	if user.CurrentWaters < maxWatering {
		return true
	}
	// Otherwise, do not allow.
	return false
}

func UpdateCache(screenName string) {
	user, ok := cache[screenName]
	if !ok {
		// If user was not in cache, initialize them.
		cache[screenName] = User{
			ScreenName:    screenName,
			CurrentWaters: 1,
			TotalWaters:   1,
			LastWatered:   time.Now(),
		}
	}

	// If user is in cache, update.
	cache[screenName] = User{
		ScreenName:    user.ScreenName,
		CurrentWaters: user.CurrentWaters + 1,
		TotalWaters:   user.TotalWaters + 1,
		LastWatered:   time.Now(),
	}
}

func ResetCache() {
	cache = map[string]User{}
}
