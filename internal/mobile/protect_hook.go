package mobile
import "sync/atomic"
type ProtectFunc func(fd int32) bool
var globalProtectFn atomic.Pointer[ProtectFunc]
func SetProtectCallback(fn ProtectFunc) {
if fn == nil { globalProtectFn.Store(nil) } else { globalProtectFn.Store(&fn) }
}
func ProtectFD(fd int32) bool {
ptr := globalProtectFn.Load(); if ptr == nil { return true }; return (*ptr)(fd)
}
