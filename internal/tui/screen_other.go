//go:build !unix

package tui

func onResize(fn func()) (stop func()) { return func() {} }
