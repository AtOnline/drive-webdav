package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/AtOnline/drive-webdav/oauth2"
	"golang.org/x/net/webdav"
)

type HttpServer struct {
	webdav.Handler
	l *net.TCPListener
}

const (
	authEP      = "https://hub.atonline.com/_special/rest/OAuth2:auth"
	tokenEP     = "https://hub.atonline.com/_special/rest/OAuth2:token"
	clientId    = "oaap-k4ch3u-kibn-bovo-cb6t-uf463ufi"
	redirectUri = "http://localhost:50500/_login"
)

func NewHttpServer() (*HttpServer, error) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50500})
	if err != nil {
		return nil, err
	}
	res := &HttpServer{l: l}
	o, err := oauth2.FromDisk(clientId, tokenEP)
	if err != nil {
		log.Printf("Failed to load token from disk: %s", err)
	} else if o != nil {
		res.Handler.FileSystem = NewDriveFS(o)
		res.Handler.FileSystem.Stat(context.TODO(), "/")
	} else {
		res.Handler.FileSystem = NewDriveLoginFS(res, res.LoginUrl(), "Click here to Login")
		log.Printf("login url: %s", res.LoginUrl())
	}
	res.Handler.LockSystem = webdav.NewMemLS()
	res.Handler.Logger = func(r *http.Request, err error) {
		if err != nil {
			log.Printf("webdav: %s", err)
		}
	}

	return res, err
}

func (h *HttpServer) Serve() error {
	return http.Serve(h.l, h)
}

func (h *HttpServer) String() string {
	return h.l.Addr().String()
}

func (h *HttpServer) LoginUrl() string {
	loginUrl := authEP + "?response_type=code&client_id=" + url.QueryEscape(clientId) + "&redirect_uri=" + url.QueryEscape(redirectUri) + "&scope=profile+Drive"
	return loginUrl
}

func (h *HttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/_login":
			c, err := oauth2.NewOAuth2(tokenEP, clientId, redirectUri, r.URL.Query().Get("code"))
			if err != nil {
				fmt.Fprintf(w, "Error authenticating: %s", err)
				return
			}
			fs := NewDriveFS(c)
			h.Handler.FileSystem = fs
			h.Handler.LockSystem = webdav.NewMemLS() // TODO
			fmt.Fprintf(w, "READY, you can now browse dav://%s", h)
			return
		case "/_log":
			LogDmesg(w)
			return
		}
	}
	h.Handler.ServeHTTP(w, r)
}

func (h *HttpServer) Stop() {
	h.l.Close()
}
