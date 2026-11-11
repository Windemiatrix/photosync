// Package app holds the use cases of photosync. It declares the interfaces of
// the systems outside the process that the use cases drive, next to their
// only user: an adapter implements one without importing app, so the types of
// the signatures and the errors that app tells apart live in domain.
package app
