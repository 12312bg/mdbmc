package bmc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrNotSupported means no enabled protocol can perform the operation.
var ErrNotSupported = errors.New("该设备不支持此操作")

// friendly converts low-level protocol errors into short Chinese messages
// while keeping the original text for troubleshooting.
func friendly(proto Protocol, err error) error {
	if err == nil {
		return nil
	}
	var mm *FingerprintMismatchError
	if errors.As(err, &mm) {
		return err
	}
	label := map[Protocol]string{ProtocolIPMI: "IPMI", ProtocolRedfish: "Redfish"}[proto]
	msg := strings.ToLower(err.Error())
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s: 操作已取消", label)
	case errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout(),
		strings.Contains(msg, "timeout"), strings.Contains(msg, "timed out"):
		return fmt.Errorf("%s: 连接超时，请检查地址、端口与网络 (%w)", label, err)
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("%s: 连接被拒绝 (%w)", label, err)
	case strings.Contains(msg, "no route to host"), strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "host is down"):
		return fmt.Errorf("%s: 网络不可达 (%w)", label, err)
	case strings.Contains(msg, "401"), strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "rakp"), strings.Contains(msg, "integrity check"),
		strings.Contains(msg, "invalid name"), strings.Contains(msg, "password"):
		return fmt.Errorf("%s: 认证失败，请检查用户名和密码 (%w)", label, err)
	case strings.Contains(msg, "privilege"):
		return fmt.Errorf("%s: BMC 账号权限不足 (%w)", label, err)
	}
	return fmt.Errorf("%s: %w", label, err)
}

// joinErrors renders several protocol errors into one message.
func joinErrors(errs []error) error {
	var parts []string
	for _, e := range errs {
		if e != nil {
			parts = append(parts, e.Error())
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return errors.New(parts[0])
	}
	return errors.New(strings.Join(parts, "；"))
}
