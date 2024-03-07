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
 *
 * API version: 1.0.0
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	base "github.com/Cray-HPE/hms-base"
	hmetcd "github.com/Cray-HPE/hms-hmetcd"
	"github.com/OpenCHAMI/bss/internal/postgres"
)

const kvDefaultRetryCount uint64 = 10
const kvDefaultRetryWait uint64 = 5
const sqlDefaultRetryCount uint64 = 10
const sqlDefaultRetryWait uint64 = 5
const authDefaultRetryCount uint64 = 10

var (
	httpListen    = ":27778"
	datastoreBase = "" // If using ETCD
	sqlHost       = "localhost"
	sqlPort       = uint(5432)
	kvHost        = ""
	kvPort        = ""
	kvRetryCount  = kvDefaultRetryCount
	kvRetryWait   = kvDefaultRetryWait
	notifierURL   = ""
	bssdb         postgres.BootDataDatabase
	bssdbName     = "bssdb"
	sqlUser       = "bssuser"
	sqlPass       = "bssuser"
	hsmBase       = "http://localhost:27779"
	nfdBase       = "http://localhost:28600"
	serviceName   = "boot-script-service"
	// This should be the IP to reach this service. Usually the external Istio
	// API gateway IP. We need to use an IP because we can't guarantee we have
	// DNS yet. This IP is passed into a booting node via the kernel params.
	// Ideally we have a static link local address that is well defined here.
	// However, our current networking does not allow this at the L2 level.
	// TODO: Set the default to a well known link local address when we have it.
	// This will also mean we change the virtual service into an Ingress with
	// this well known IP.
	advertiseAddress  = "" // i.e. http://{IP to reach this service}
	insecure          = false
	debugFlag         = false
	kvstore           hmetcd.Kvi
	retryDelay        = uint(30)
	hsmRetrievalDelay = uint(10)
	sqlRetryCount     = sqlDefaultRetryCount
	sqlRetryWait      = sqlDefaultRetryWait
	notifier          *ScnNotifier
	useSQL            = false // Use ETCD by default
	authRetryCount    = authDefaultRetryCount
	jwksURL           = ""
	sqlDbOpts         = ""
	spireServiceURL   = "https://spire-tokens.spire:54440"
)

func parseEnv(evar string, v interface{}) (ret error) {
	if val := os.Getenv(evar); val != "" {
		switch vp := v.(type) {
		case *int:
			var temp int64
			temp, ret = strconv.ParseInt(val, 0, 64)
			if ret == nil {
				*vp = int(temp)
			}
		case *uint:
			var temp uint64
			temp, ret = strconv.ParseUint(val, 0, 64)
			if ret == nil {
				*vp = uint(temp)
			}
		case *string:
			*vp = val
		case *bool:
			switch strings.ToLower(val) {
			case "0", "off", "no", "false":
				*vp = false
			case "1", "on", "yes", "true":
				*vp = true
			default:
				ret = fmt.Errorf("Unrecognized bool value: '%s'", val)
			}
		case *[]string:
			*vp = strings.Split(val, ",")
		default:
			ret = fmt.Errorf("Invalid type for receiving ENV variable value %T", v)
		}
	}
	return
}

func debugf(format string, v ...interface{}) {
	if debugFlag {
		log.Printf("DEBUG: "+format, v...)
	}
}

func kvDefaultURL() string {
	ret := "mem:"
	if kvHost != "" && kvPort != "" {
		ret = "http://" + kvHost + ":" + kvPort
	}
	return ret
}

func kvDefaultRetryConfig() (retryCount uint64, retryWait uint64, err error) {
	retryCount = kvDefaultRetryCount
	retryWait = kvDefaultRetryWait

	envRetryCount := os.Getenv("ETCD_RETRY_COUNT")
	if envRetryCount != "" {
		retryCount, err = strconv.ParseUint(envRetryCount, 10, 64)
		if err != nil {
			log.Println("ERROR enable to parse ETCD_RETRY_COUNT environment variable: ", err)
			return kvDefaultRetryCount, kvDefaultRetryWait, err
		}
	}

	envRetryWait := os.Getenv("ETCD_RETRY_WAIT")
	if envRetryWait != "" {
		retryWait, err = strconv.ParseUint(envRetryWait, 10, 64)
		if err != nil {
			log.Println("ERROR enable to parse ETCD_RETRY_WAIT environment variable: ", err)
			return kvDefaultRetryCount, kvDefaultRetryWait, err
		}
	}

	return retryCount, retryWait, nil
}

func kvOpen(url, opts string, retryCount, retryWait uint64) (err error) {
	ix := uint64(1)
	for ; ix <= retryCount; ix++ {
		log.Println("Attempting connection to ETCD (attempt ", ix, ")")
		kvstore, err = hmetcd.Open(url, opts)
		if err != nil {
			log.Println("ERROR opening connection to ETCD (attempt ", ix, "):", err)
		} else {
			break
		}

		time.Sleep(time.Duration(retryWait) * time.Second)
	}
	if ix > retryCount {
		err = fmt.Errorf("ETCD connection attempts exhausted (%d).", retryCount)
	} else {
		log.Printf("KV service initialized connecting to %s", url)
	}
	return err
}

func sqlOpen(host string, port uint, user, password, extraDbOpts string, ssl bool, retryCount, retryWait uint64) (postgres.BootDataDatabase, error) {
	var (
		err  error
		bddb postgres.BootDataDatabase
	)
	ix := uint64(1)

	for ; ix <= retryCount; ix++ {
		log.Printf("Attempting connection to Postgres (attempt %d)", ix)
		bddb, err = postgres.Connect(host, port, bssdbName, user, password, ssl, extraDbOpts)
		if err != nil {
			log.Printf("ERROR opening opening connection to Postgres (attempt %d): %v\n", ix, err)
		} else {
			break
		}

		time.Sleep(time.Duration(retryWait) * time.Second)
	}
	if ix > retryCount {
		err = fmt.Errorf("Postgres connection attempts exhausted (%d).", retryCount)
	} else {
		log.Printf("Initialized connection to Postgres database at %s:%d", host, port)
	}
	return bddb, err
}

func sqlClose() {
	err := bssdb.Close()
	if err != nil {
		log.Fatalf("Error attempting tp close connection to Postgres: %v", err)
	}
}

func getNotifierURL() string {
	if notifierURL == "" {
		var err error
		notifierURL, err = os.Hostname()
		if err == nil {
			if strings.Contains(notifierURL, "cray-bss") {
				notifierURL = "cray-bss"
			} else {
				notifierURL += httpListen
			}
		} else {
			// If all else fails, use localhost
			// This may not work for NFD, but things are kind of
			// messed up anyway if you can't get the hostname.
			log.Printf("Could not get hostname: %s", err)
			notifierURL = "localhost" + httpListen
		}
	}
	url := "http://" + notifierURL + notifierEndpoint
	log.Printf("Notification endpoint: %s", url)
	return url
}

func parseEnvVars() error {
	var (
		err      error = nil
		parseErr error
		errList  []error
	)

	//
	// General BSS environment variables
	//

	parseErr = parseEnv("BSS_SERVICE_NAME", &serviceName)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_SERVICE_NAME: %q", parseErr))
	}
	parseErr = parseEnv("BSS_HTTP_LISTEN", &httpListen)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_HTTP_LISTEN: %q", parseErr))
	}
	parseErr = parseEnv("HSM_URL", &hsmBase)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("HSM_URL: %q", parseErr))
	}
	parseErr = parseEnv("NFD_URL", &nfdBase)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("NFD_URL: %q", parseErr))
	}
	parseErr = parseEnv("BSS_ADVERTISE_ADDRESS", &advertiseAddress)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_ADVERTISE_ADDRESS: %q", parseErr))
	}
	parseErr = parseEnv("BSS_RETRY_DELAY", &retryDelay)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_RETRY_DELAY: %q", parseErr))
	}
	parseErr = parseEnv("BSS_HSM_RETRIEVAL_DELAY", &hsmRetrievalDelay)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_HSM_RETRIEVAL_DELAY: %q", parseErr))
	}
	parseErr = parseEnv("SPIRE_TOKEN_URL", &spireServiceURL)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("SPIRE_TOKEN_URL: %q", parseErr))
	}
	parseErr = parseEnv("BSS_ENDPOINT_HOST", &notifierURL)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_ENDPOINT_HOST: %q", parseErr))
	}
	parseErr = parseEnv("BSS_AUTH_RETRY_COUNT", &authRetryCount)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_AUTH_RETRY_COUNT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_JWKS_URL", &jwksURL)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_JWKS_URL: %q", parseErr))
	}

	//
	// Etcd environment variables
	//

	parseErr = parseEnv("DATASTORE_BASE", &datastoreBase)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("DATASTORE_BASE: %q", parseErr))
	}
	parseErr = parseEnv("ETCD_HOST", &kvHost)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("ETCD_HOST: %q", parseErr))
	}
	parseErr = parseEnv("ETCD_PORT", &kvPort)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("ETCD_PORT: %q", parseErr))
	}
	parseErr = parseEnv("ETCD_RETRY_COUNT", &kvRetryCount)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("ETCD_RETRY_COUNT: %q", parseErr))
	}
	parseErr = parseEnv("ETCD_RETRY_WAIT", &kvRetryWait)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("ETCD_RETRY_WAIT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_INSECURE", &insecure)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_INSECURE: %q", parseErr))
	}

	//
	// SQL environment variables
	//

	parseErr = parseEnv("BSS_USESQL", &useSQL)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_USESQL: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DEBUG", &debugFlag)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DEBUG: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBHOST", &sqlHost)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBHOST: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBPORT", &sqlPort)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBPORT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBNAME", &bssdbName)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBNAME: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBOPTS", &sqlDbOpts)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBOPTS: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBUSER", &sqlUser)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBUSER: %q", parseErr))
	}
	parseErr = parseEnv("BSS_DBPASS", &sqlPass)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_DBPASS: %q", parseErr))
	}
	parseErr = parseEnv("BSS_SQL_RETRY_COUNT", &sqlRetryCount)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_SQL_RETRY_COUNT: %q", parseErr))
	}
	parseErr = parseEnv("BSS_SQL_RETRY_WAIT", &sqlRetryWait)
	if parseErr != nil {
		errList = append(errList, fmt.Errorf("BSS_SQL_RETRY_WAIT: %q", parseErr))
	}

	if len(errList) > 0 {
		err = fmt.Errorf("Error(s) parsing environment variables: %v", errList)
	}

	return err
}

func parseCmdLine() {
	flag.StringVar(&httpListen, "http-listen", httpListen, "(BSS_HTTP_LISTEN) HTTP server IP + port binding")
	flag.StringVar(&hsmBase, "hsm", hsmBase, "(HSM_URL) Hardware State Manager location as URI, e.g. [scheme]://[host[:port]]")
	flag.StringVar(&nfdBase, "nfd", nfdBase, "(NFD_URL) Notification daemon location as URI, e.g. [scheme]://[host[:port]]")
	flag.StringVar(&datastoreBase, "datastore", kvDefaultURL(), "(DATASTORE_BASE) Datastore Service location as URI")
	flag.StringVar(&sqlHost, "postgres-host", sqlHost, "(BSS_DBHOST) Postgres host as IP address or name")
	flag.StringVar(&serviceName, "service-name", serviceName, "(BSS_SERVICE_NAME) Boot script service name")
	flag.StringVar(&spireTokensBaseURL, "spire-url", spireServiceURL, "(SPIRE_TOKEN_URL) Spire join token service base URL")
	flag.StringVar(&advertiseAddress, "cloud-init-address", advertiseAddress, "(BSS_ADVERTISE_ADDRESS) IP:PORT to advertise for cloud-init calls. This needs to be an IP as we do not have DNS when cloud-init runs")
	flag.StringVar(&bssdbName, "postgres-dbname", bssdbName, "(BSS_DBNAME) Postgres database name")
	flag.StringVar(&sqlUser, "postgres-username", sqlUser, "(BSS_DBUSER) Postgres username")
	flag.StringVar(&sqlPass, "postgres-password", sqlPass, "(BSS_DBPASS) Postgres password")
	flag.StringVar(&jwksURL, "jwks-url", jwksURL, "(BSS_JWKS_URL) Set the JWKS URL to fetch the public key for authorization (enables authentication)")
	flag.BoolVar(&insecure, "insecure", insecure, "(BSS_INSECURE) Don't enforce https certificate security")
	flag.BoolVar(&debugFlag, "debug", debugFlag, "(BSS_DEBUG) Enable debug output")
	flag.BoolVar(&useSQL, "postgres", useSQL, "(BSS_USESQL) Use Postgres instead of ETCD")
	flag.UintVar(&retryDelay, "retry-delay", retryDelay, "(BSS_RETRY_DELAY) Retry delay in seconds")
	flag.UintVar(&hsmRetrievalDelay, "hsm-retrieval-delay", hsmRetrievalDelay, "(BSS_HSM_RETRIEVAL_DELAY) SM Retrieval delay in seconds")
	flag.UintVar(&sqlPort, "postgres-port", sqlPort, "(BSS_DBPORT) Postgres port")
	flag.Uint64Var(&authRetryCount, "auth-retry-count", authRetryCount, "(BSS_AUTH_RETRY_COUNT) Retry fetching JWKS public key set")
	flag.Uint64Var(&sqlRetryCount, "postgres-retry-count", sqlRetryCount, "(BSS_SQL_RETRY_COUNT) Amount of times to retry connecting to Postgres")
	flag.Uint64Var(&sqlRetryWait, "postgres-retry-wait", sqlRetryCount, "(BSS_SQL_RETRY_WAIT) Interval in seconds between connection attempts to Postgres")
	flag.Parse()
}

func main() {
	err := parseEnvVars()
	if err != nil {
		log.Println(err)
		log.Println("WARNING: Ignoring environment variables with errors.")
	}
	parseCmdLine()

	dumb := someFuncDoesNotExist()

	sn, snerr := base.GetServiceInstanceName()
	if snerr == nil {
		serviceName = sn
	}
	log.Printf("Service %s started", serviceName)

	router := initHandlers()

	// try and fetch JWKS from issuer
	if jwksURL != "" {
		for i := uint64(0); i <= authRetryCount; i++ {
			err := loadPublicKeyFromURL(jwksURL)
			if err != nil {
				log.Printf("failed to initialize auth token: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Printf("Initialized the auth token successfully.")
			break
		}
	}

	var svcOpts string
	if insecure {
		svcOpts = "insecure,"
	}
	if debugFlag {
		svcOpts += "debug"
	}

	if advertiseAddress == "" {
		log.Fatalf("--cloud-init-address or BSS_ADVERTISE_ADDRESS required.")
	}

	err = SmOpen(hsmBase, svcOpts)
	if err != nil {
		log.Fatalf("Access to SM service %s failed: %v\n", hsmBase, err)
	}

	notifier = newNotifier(serviceName, nfdBase+"/hmi/v1/subscribe", getNotifierURL(), svcOpts)

	// If --postgres passed, use Postgres. Otherwise, use Etcd.
	if useSQL {
		log.Printf("sqlRetryCount=%d sqlRetryWait=%ds", sqlRetryCount, sqlRetryWait)

		// Initiate connection to Postgres.
		log.Printf("Using insecure connection to SQL database: %v\n", insecure)
		bssdb, err = sqlOpen(sqlHost, sqlPort, sqlUser, sqlPass, sqlDbOpts, !insecure, sqlRetryCount, sqlRetryWait)
		if err != nil {
			log.Fatalf("Access to Postgres database at %s:%d failed: %v\n", sqlHost, sqlPort, err)
		}
		defer sqlClose()
	} else {
		err = kvOpen(datastoreBase, svcOpts, kvRetryCount, kvRetryWait)
		if err != nil {
			log.Fatalf("Access to Datastore service %s with name %s failed: %v\n", datastoreBase, serviceName, err)
		}
	}
	err = spireTokenServiceInit(spireServiceURL, svcOpts)
	if err != nil {
		// NOTE: Should this be fatal???  Right now, we will continue.
		log.Printf("WARNING: Spire join token service %s access failure: %s", spireServiceURL, err)
	}
	log.Fatal(http.ListenAndServe(httpListen, router))
}
