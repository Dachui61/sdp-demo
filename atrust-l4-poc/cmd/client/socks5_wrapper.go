package main

import (
	"net"
	"time"
)

// WrappedConn 包装net.Conn以提供TCPAddr兼容的LocalAddr方法
type WrappedConn struct {
	net.Conn
}

// LocalAddr 返回包装的本地地址，如果原始连接不提供TCP地址，则返回一个虚拟的TCP地址
func (wc *WrappedConn) LocalAddr() net.Addr {
	if tcpAddr, ok := wc.Conn.LocalAddr().(*net.TCPAddr); ok {
		return tcpAddr
	}
	// 如果不是TCP地址，返回一个虚拟的TCP地址
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

// RemoteAddr 返回包装的远程地址，如果原始连接不提供TCP地址，则返回一个虚拟的TCP地址
func (wc *WrappedConn) RemoteAddr() net.Addr {
	if tcpAddr, ok := wc.Conn.RemoteAddr().(*net.TCPAddr); ok {
		return tcpAddr
	}
	// 如果不是TCP地址，返回一个虚拟的TCP地址
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

// SetDeadline 设置读写截止时间
func (wc *WrappedConn) SetDeadline(t time.Time) error {
	return wc.Conn.SetDeadline(t)
}

// SetReadDeadline 设置读截止时间
func (wc *WrappedConn) SetReadDeadline(t time.Time) error {
	return wc.Conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写截止时间
func (wc *WrappedConn) SetWriteDeadline(t time.Time) error {
	return wc.Conn.SetWriteDeadline(t)
}

// NewWrappedConn 创建一个新的包装连接
func NewWrappedConn(conn net.Conn) net.Conn {
	return &WrappedConn{
		Conn: conn,
	}
}