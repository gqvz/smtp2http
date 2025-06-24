package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/alash3al/go-smtpsrv"
	"github.com/go-resty/resty/v2"
)

func main() {
	cfg := smtpsrv.ServerConfig{
		ReadTimeout:     time.Duration(*flagReadTimeout) * time.Second,
		WriteTimeout:    time.Duration(*flagWriteTimeout) * time.Second,
		ListenAddr:      *flagListenAddr,
		MaxMessageBytes: int(*flagMaxMessageSize),
		BannerDomain:    *flagServerName,
		Handler: smtpsrv.HandlerFunc(func(c *smtpsrv.Context) error {
			msg, err := c.Parse()
			if err != nil {
				return errors.New("Cannot read your message: " + err.Error())
			}

			// this is completely chatgpt'd
			from := transformStdAddressToEmailAddress([]*mail.Address{c.From()})[0]
			to := transformStdAddressToEmailAddress([]*mail.Address{c.To()})[0]

			cc := transformStdAddressToEmailAddress(msg.Cc)
			bcc := transformStdAddressToEmailAddress(msg.Bcc)

			apiKey := *flagMailgunAPIKey
			apiUrl := *flagWebhook

			client := resty.New()
			request := client.R().
				SetBasicAuth("api", apiKey).
				SetHeader("Content-Type", "multipart/form-data")

			form := map[string]string{
				"from":    fmt.Sprintf("%s <%s>", from.Name, from.Address),
				"to":      to.Address,
				"subject": msg.Subject,
				"text":    string(msg.TextBody),
				"html":    string(msg.HTMLBody),
			}

			// Optional fields
			if len(cc) > 0 {
				var ccList []string
				for _, a := range cc {
					ccList = append(ccList, a.Address)
				}
				form["cc"] = strings.Join(ccList, ",")
			}
			if len(bcc) > 0 {
				var bccList []string
				for _, a := range bcc {
					bccList = append(bccList, a.Address)
				}
				form["bcc"] = strings.Join(bccList, ",")
			}

			if len(msg.ReplyTo) > 0 {
				var replyTos []string
				for _, rt := range msg.ReplyTo {
					if rt.Name != "" {
						replyTos = append(replyTos, fmt.Sprintf("%s <%s>", rt.Name, rt.Address))
					} else {
						replyTos = append(replyTos, rt.Address)
					}
				}
				form["h:Reply-To"] = strings.Join(replyTos, ", ")
			}

			if len(msg.InReplyTo) > 0 {
				form["h:In-Reply-To"] = strings.Join(msg.InReplyTo, " ")
			}

			if len(msg.References) > 0 {
				form["h:References"] = strings.Join(msg.References, " ")
			}

			request.SetFormData(form)

			for _, a := range msg.Attachments {
				data, _ := ioutil.ReadAll(a.Data)
				request.SetFileReader("attachment", a.Filename, strings.NewReader(string(data)))
			}

			for _, a := range msg.EmbeddedFiles {
				data, _ := ioutil.ReadAll(a.Data)
				request.SetFileReader("inline", a.CID, strings.NewReader(string(data)))
				form["inline"] = a.CID
			}

			// Send request
			resp, err := request.Post(apiUrl)
			if err != nil {
				log.Println(err)
				return errors.New("e1: Cannot send via Mailgun")
			} else if resp.StatusCode() >= 300 {
				log.Printf("Mailgun error: %s - %s\n", resp.Status(), string(resp.Body()))
				return errors.New("e2: Mailgun rejected the message")
			}

			return nil
		}),
	}

	fmt.Println(smtpsrv.ListenAndServe(&cfg))
}
