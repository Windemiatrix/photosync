// Package clock is the only place outside cmd and config that reads the real
// time and waits. The app and the adapters receive a Clock, so tests move time
// with Fake instead of sleeping; the domain gets the current time as a
// parameter.
package clock
