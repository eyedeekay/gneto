// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// geminiToHTML reads Gemini text from rd, and writes its HTML equivalent to w.
// The source URL is stored in u.
func geminiToHTML(w http.ResponseWriter, u *url.URL, rd *bufio.Reader) error {
	var err error
	var html string
	list := false
	pre := false

	var td templateData
	if err != nil {
		td.Error = err.Error()
	}
	td.URL = u.String()
	td.Title = "Gneto " + td.URL

	err = tmpls.ExecuteTemplate(w, "header-only.html.tmpl", td)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", 500)
	}

	var eof error
	var line string
	for eof == nil {
		line, eof = rd.ReadString("\n"[0])
		if optDebug {
			fmt.Println(line)
		}

		if reGemPre.MatchString(line) {
			if pre == true {
				pre = false
				io.WriteString(w, "</pre>\n")
				continue
			} else {
				pre = true
				// How can we provide alt text from reGemPre.FindStringSubmatch(line)[1]?
				io.WriteString(w, "<pre>\n")
				continue
			}
		} else {
			if pre == true {
				io.WriteString(w, strings.ReplaceAll(line, "<", "&lt;"))
				continue
			}
		}

		if reGemBlank.MatchString(line) {
			io.WriteString(w, "<br>\n")
		} else if reGemH1.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, html+"<h1>"+reGemH1.FindStringSubmatch(line)[1]+"</h1>\n")
		} else if reGemH2.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, html+"<h2>"+reGemH2.FindStringSubmatch(line)[1]+"</h2>\n")
		} else if reGemH3.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, html+"<h3>"+reGemH3.FindStringSubmatch(line)[1]+"</h3>\n")
		} else if reGemLink.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}

			link := reGemLink.FindStringSubmatch(line)
			lineURL, err := absoluteURL(u, link[1])
			if err != nil {
				io.WriteString(w, html+"<p>"+line+"</p>\n")
			}
			link[1] = lineURL.String()

			if lineURL.Scheme == "gemini" {
				if link[2] != "" {
					io.WriteString(w, html+`<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				} else {
					io.WriteString(w, html+`<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				}
			} else {
				if link[2] != "" {
					io.WriteString(w, html+`<p><a href="`+link[1]+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				} else {
					io.WriteString(w, html+`<p><a href="`+link[1]+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				}
			}
		} else if reGemList.MatchString(line) {
			if list == false {
				list = true
				io.WriteString(w, "<ul>")
			}
			io.WriteString(w, html+"<li>"+reGemList.FindStringSubmatch(line)[1]+"</li>\n")
		} else if reGemQuote.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>")
			}
			io.WriteString(w, html+"<blockquote>"+reGemQuote.FindStringSubmatch(line)[1]+"</blockquote>\n")
		} else {
			io.WriteString(w, line+"<br>\n")
		}
	}

	err = tmpls.ExecuteTemplate(w, "footer-only.html.tmpl", td)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", 500)
	}

	return err
}

// proxyGemini finds the Gemini content at u.
func proxyGemini(w http.ResponseWriter, u *url.URL) (*url.URL, error) {
	var err error
	var rd *bufio.Reader

	var port string
	if u.Port() != "" {
		port = u.Port()
	} else {
		port = "1965"
	}

	conn, err := tls.Dial("tcp", u.Hostname()+":"+port, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return u, fmt.Errorf("proxyGemini: tls.Dial error to %s: %v", u.String(), err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, u.String()+"\r\n")

	rd = bufio.NewReader(conn)

	status, err := rd.ReadString("\n"[0])
	if err != nil {
		return u, fmt.Errorf("proxyGemini: failed to read status line from buffer: %v", err)
	}
	if optDebug {
		log.Printf("proxyGemini: %s status: %s", u.String(), status)
	}
	if !reStatus.MatchString(status) {
		return u, fmt.Errorf("proxyGemini: invalid status line: %s", status)
	}

	switch status[0] {
	case "1"[0]:
		// TODO: Get user input.
	case "2"[0]:
		if strings.Contains(status, "text/gemini") {
			geminiToHTML(w, u, rd)
		} else {
			log.Printf("proxyGemini: MIME type not text/gemini: '%s'", status)
		}
	case "3"[0]:
		ru, err := url.Parse(strings.TrimSpace(strings.SplitAfterN(status, " ", 2)[1]))
		if err != nil {
			return u, fmt.Errorf("proxyGemini: can't parse redirect URL %s: %v", strings.SplitAfterN(status, " ", 2)[1], err)
		}
		if ru.Host == "" {
			ru.Host = u.Host
		}
		if ru.Scheme == "" {
			ru.Scheme = u.Scheme
		}
		errRedirect = errors.New(u.String())
		err = errRedirect
		return ru, err
	default:
		return u, fmt.Errorf("proxyGemini: status: %s", status)
	}

	return u, err
}
