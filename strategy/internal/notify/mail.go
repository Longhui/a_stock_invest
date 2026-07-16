package notify

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
)

// MailConfig SMTP 邮件配置
type MailConfig struct {
	Host string
	Port string
	User string
	Pass string
	To   string
}

// EnvMailConfig 返回邮件配置
func EnvMailConfig() *MailConfig {
	return &MailConfig{
		Host: "smtp.163.com",
		Port: "465",
		User: "mystock666@163.com",
		Pass: "RJdvWhm7c9reVeCT",
		To:   "mystock666@163.com",
	}
}

// SendMail 发送邮件
func SendMail(cfg *MailConfig, subject, body string) error {
	auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)

	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		cfg.To, subject, body))

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	// 465端口 → SSL/TLS 直连；其他端口(默认587) → STARTTLS
	if cfg.Port == "465" {
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("TLS连接失败: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP客户端创建失败: %w", err)
		}
		defer client.Close()

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP认证失败: %w", err)
		}
		if err = client.Mail(cfg.User); err != nil {
			return fmt.Errorf("发件人地址失败: %w", err)
		}
		if err = client.Rcpt(cfg.To); err != nil {
			return fmt.Errorf("收件人地址失败: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("写入邮件数据失败: %w", err)
		}
		if _, err = w.Write(msg); err != nil {
			return fmt.Errorf("写入邮件内容失败: %w", err)
		}
		if err = w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}

	// 默认 STARTTLS (587)
	return smtp.SendMail(addr, auth, cfg.User, []string{cfg.To}, msg)
}
