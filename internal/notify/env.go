package notify

import "os"

// lookupEnv returns the value of an environment variable, or "" if not set.
func lookupEnv(key string) string {
	return os.Getenv(key)
}
