package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
)

type email struct {
	SenderAddress   string
	TargetAddresses []string
	Subject         string
	Body            string
}

func (mail *email) AddRecipient(address string) {
	mail.TargetAddresses = append(mail.TargetAddresses, address)
}

func (mail *email) buildMessage() []byte {
	bodyHash := sha256.Sum256([]byte(mail.Body))
	hashString := hex.EncodeToString(bodyHash[:])

	msg := fmt.Sprintf("To: <%s>\r\n", strings.Join(mail.TargetAddresses, ">; <"))
	msg += fmt.Sprintf("From: <%s>\r\n", mail.SenderAddress)
	msg += fmt.Sprintf("Subject: %s\r\n", mail.Subject)
	msg += fmt.Sprintf("Message-ID: <%s@kimmunity.se>\r\n", hashString)
	msg += "MIME-Version: 1.0\r\n"
	msg += "Content-Type: text/html; charset=UTF-8\r\n"
	msg += "Content-Transfer-Encoding: 8bit\r\n"
	msg += "\r\n" + mail.Body

	return []byte(msg)
}

func (mail *email) Send() error {
	smtpServer := fmt.Sprintf("%s:%v", settings.Email.Server, settings.Email.Port)
	tlsconfig := &tls.Config{InsecureSkipVerify: true, ServerName: settings.Email.Server}
	auth := smtp.PlainAuth("", settings.Email.Username, settings.Email.Password, settings.Email.Server)
	client, err := smtp.Dial(smtpServer)

	if err != nil {
		return err
	}

	if err := client.StartTLS(tlsconfig); err != nil {
		return err
	}

	if err = client.Auth(auth); err != nil {
		return err
	}

	if err = client.Mail(mail.SenderAddress); err != nil {
		return err
	}
	for _, c := range mail.TargetAddresses {
		if err = client.Rcpt(c); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = w.Write(mail.buildMessage())
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}

	client.Quit()
	return nil
}
