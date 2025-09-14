package env

import (
	"fmt"
	"os"
	"strconv"
)

// GetString extracts a String value from the given environment variable
func GetString(name string, defaultValue ...string) string {
	value := os.Getenv(name)
	if value == "" && len(defaultValue) > 0 {
		value = defaultValue[0]
	}
	return value
}

// MustGetString extracts a String value from the given environment variable
// It panics if the environment variable is not present
func MustGetString(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic(fmt.Sprintf("%s can't be empty", name))
	}
	return value
}

// GetInt extracts an Int value from the given environment variable
func GetInt(name string, defaultValue ...int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil && len(defaultValue) > 0 {
		value = defaultValue[0]
	}
	return value
}

// MustGetInt extracts an Int value from the given environment variable
// It panics if the environment variable is not present or not an integer
func MustGetInt(name string) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		panic(fmt.Sprintf("%s must contain a int value!", name))
	}
	return value
}

// GetInt32 extracts an Int value from the given environment variable
func GetInt32(name string, defaultValue ...int32) int32 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 32)
	if err != nil && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return int32(value)
}

// MustGetInt32 extracts an Int value from the given environment variable
// It panics if the environment variable is not present or not an integer
func MustGetInt32(name string) int32 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 32)
	if err != nil {
		panic(fmt.Sprintf("%s must contain a int32 value!", name))
	}
	return int32(value)
}

// GetFloat extracts a Float value from the given environment variable
func GetFloat(name string, defaultValue ...float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil && len(defaultValue) > 0 {
		value = defaultValue[0]
	}
	return value
}

// MustGetFloat extracts a Float value from the given environment variable
// It panics if the environment variable is not present or not a float
func MustGetFloat(name string) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil {
		panic(fmt.Sprintf("%s must contain a float value!", name))
	}
	return value
}

// GetBool extracts a Bool value from the given environment variable
func GetBool(name string, defaultValue ...bool) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	if err != nil && len(defaultValue) > 0 {
		value = defaultValue[0]
	}
	return value
}

// MustGetBool extracts a Bool value from the given environment variable
// It panics if the environment variable is not present or not a boolean
func MustGetBool(name string) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	if err != nil {
		panic(fmt.Sprintf("%s must contain a boolean value! (true or false)", name))
	}
	return value
}
