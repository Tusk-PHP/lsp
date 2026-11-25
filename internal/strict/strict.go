package strict

import "os"

const EnvVar = "TUSK_STRICT"

func Enabled() bool {
	switch os.Getenv(EnvVar) {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return true
	}
	return false
}
