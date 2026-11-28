// Package config reads the configuration file of photosync. The parse is
// strict: a key that the file misspells, repeats or keeps from an earlier
// design is an error with its path and line rather than a key that is
// silently ignored. Each key that the file leaves out or leaves empty takes
// its default. The package checks no value; the packages that take a value
// check it.
package config
