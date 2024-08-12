// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
/*
 * Boot Script Server
 *
 * The boot script server will collect all information required to produce an
 * iPXE boot script for each node of a system.  This script will the be
 * generated on demand and delivered to the requesting node during an iPXE
 * boot.  The main items the script will deliver are the kernel image URL/path,
 * boot arguments, and the initrd URL/path.  Note that the kernel and initrd
 * images are specified with a URL or path.  A plain path will result in a tfpt
 * download from this server.  If a URL is provided, it can be from any
 * available service which iPXE supports, and any location that the iPXE client
 * has access to. It is not restricted to a particular Cray provided service.
 */

package main

import (
	"fmt"
	"log"
	"net/http"
	net_url "net/url"
	"os"
	"time"

	base "github.com/Cray-HPE/hms-base"
	"github.com/OpenCHAMI/jwtauth/v5"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/go-retryablehttp"
	openchami_authenticator "github.com/openchami/chi-middleware/auth"
	openchami_logger "github.com/openchami/chi-middleware/log"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

const (
	baseEndpoint     = "/boot/v1"
	notifierEndpoint = baseEndpoint + "/scn"
	// We don't use the baseEndpoint here because cloud-init doesn't like them
	metaDataRoute  = "/meta-data"
	userDataRoute  = "/user-data"
	phoneHomeRoute = "/phone-home"
)

var (
	tokenAuth *jwtauth.JWTAuth
)

func initHandlers() *chi.Mux {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.StripSlashes)
	router.Use(openchami_logger.OpenCHAMILogger(logger))
	router.Use(middleware.Timeout(60 * time.Second))
	if jwksURL != "" {
		router.Group(func(r chi.Router) {
			r.Use(
				jwtauth.Verifier(tokenAuth),
				openchami_authenticator.AuthenticatorWithRequiredClaims(tokenAuth, []string{"sub", "iss", "aud"}),
			)

			// protected routes if using auth
			r.HandleFunc(baseEndpoint+"/", Index)
			r.HandleFunc(baseEndpoint+"/bootparameters", bootParameters)
		})
	} else {
		// public routes without auth
		router.HandleFunc(baseEndpoint+"/", Index)
		router.HandleFunc(baseEndpoint+"/bootparameters", bootParameters)
	}
	// every thing else is public
	// boot
	router.HandleFunc(baseEndpoint+"/bootscript", bootScript)
	router.HandleFunc(baseEndpoint+"/hosts", hosts)
	router.HandleFunc(baseEndpoint+"/dumpstate", dumpstate)
	router.HandleFunc(baseEndpoint+"/service/status", serviceStatusResponse)
	router.HandleFunc(baseEndpoint+"/service/status/all", service)
	router.HandleFunc(baseEndpoint+"/service/version", serviceVersionResponse)
	router.HandleFunc(baseEndpoint+"/service/hsm", serviceHSMResponse)
	router.HandleFunc(baseEndpoint+"/service/storage/status", serviceStorageResponse)
	// cloud-init
	router.HandleFunc(metaDataRoute, metaDataGet)
	router.HandleFunc(userDataRoute, userDataGet)
	router.HandleFunc(phoneHomeRoute, phoneHomePost)
	// notifications
	router.HandleFunc(notifierEndpoint, scn)
	// endpoint-access
	router.HandleFunc(baseEndpoint+"/endpoint-history", endpointHistoryGet)
	return router
}

func Index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello World!")
}

func sendAllowable(w http.ResponseWriter, allowable string) {
	w.Header().Set("allow", allowable)
	base.SendProblemDetailsGeneric(w, http.StatusMethodNotAllowed, "allow "+allowable)
}

func bootParameters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		BootparametersGet(w, r)
	case http.MethodPut:
		BootparametersPut(w, r)
	case http.MethodPost:
		BootparametersPost(w, r)
	case http.MethodPatch:
		BootparametersPatch(w, r)
	case http.MethodDelete:
		BootparametersDelete(w, r)
	default:
		sendAllowable(w, "GET,PUT,POST,PATCH,DELETE")
	}
}

func bootScript(w http.ResponseWriter, r *http.Request) {
	if bootscriptNotifyURL != "" {
		go notifyTarget(bootscriptNotifyURL, r.RemoteAddr)
	}
	switch r.Method {
	case http.MethodGet:
		BootscriptGet(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func hosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		HostsGet(w, r)
	case http.MethodPost:
		HostsPost(w, r)
	default:
		sendAllowable(w, "GET,POST")
	}
}

func dumpstate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		DumpstateGet(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func service(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serviceStatusAPI(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func scn(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		stateChangeNotification(w, r)
	default:
		sendAllowable(w, "POST")
	}
}

func metaDataGet(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		metaDataGetAPI(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func userDataGet(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userDataGetAPI(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func phoneHomePost(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		phoneHomePostAPI(w, r)
	default:
		sendAllowable(w, "POST")
	}
}

func endpointHistoryGet(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		endpointHistoryGetAPI(w, r)
	default:
		sendAllowable(w, "GET")
	}
}

func notifyTarget(url string, data string) {
	resp, err := retryablehttp.PostForm(url, net_url.Values{"data": {data}})
	if err != nil {
		log.Printf("WARNING: HTTP POST of \"%v\" failed: %v\n", data, err)
		return
	}
	defer resp.Body.Close()
}
