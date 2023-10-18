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
	"github.com/Cray-HPE/hms-bss/internal/postgres"
	hmetcd "github.com/Cray-HPE/hms-hmetcd"
)

const kvDefaultRetryCount uint64 = 10
const kvDefaultRetryWait uint64 = 5
const sqlDefaultRetryCount uint64 = 10
const sqlDefaultRetryWait uint64 = 5

var (
	httpListen    = ":27778"
	datastoreBase = "" // If using ETCD
	sqlHost       = "localhost"
	sqlPort       = uint(5432)
	bssdb         postgres.BootDataDatabase
	bssdbName     = "bssdb"
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
	sqlInsecure       = false
	debugFlag         = true
	kvstore           hmetcd.Kvi
	retryDelay        = uint(30)
	hsmRetrievalDelay = uint(10)
	notifier          *ScnNotifier
	useSQL            = false // Use ETCD by default
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
	eh := os.Getenv("ETCD_HOST")
	ep := os.Getenv("ETCD_PORT")
	ret := "mem:"
	if eh != "" && ep != "" {
		ret = "http://" + eh + ":" + ep
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

// Try to read SQL_RETRY_COUNT and SQL_RETRY_WAIT environment variables.
// If either variable contains an invalid value, return the default values of both.
func sqlDefaultRetryConfig() (retryCount uint64, retryWait uint64, err error) {
	retryCount = sqlDefaultRetryCount
	retryWait = sqlDefaultRetryWait

	envRetryCount := os.Getenv("SQL_RETRY_COUNT")
	if envRetryCount != "" {
		retryCount, err = strconv.ParseUint(envRetryCount, 10, 64)
		if err != nil {
			err = fmt.Errorf("ERROR: unable to parse SQL_RETRY_COUNT environment variable: ", err)
			return kvDefaultRetryCount, kvDefaultRetryWait, err
		}
	}

	envRetryWait := os.Getenv("SQL_RETRY_WAIT")
	if envRetryWait != "" {
		retryWait, err = strconv.ParseUint(envRetryWait, 10, 64)
		if err != nil {
			err = fmt.Errorf("ERROR: unable to parse SQL_RETRY_WAIT environment variable: ", err)
			return kvDefaultRetryWait, kvDefaultRetryWait, err
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

func sqlGetCreds() (user, password string, err error) {
	err = nil
	user = os.Getenv("SQL_USER")
	if user == "" {
		err = fmt.Errorf("ERROR: unable to get SQL_USER environment variable.")
		return "", "", err
	}

	password = os.Getenv("SQL_PASSWORD")
	if password == "" {
		err = fmt.Errorf("ERROR: unable to get SQL_PASSWORD environment variable.")
		return "", "", err
	}

	return user, password, err
}

func sqlOpen(host string, port uint, user, password string, ssl bool, retryCount, retryWait uint64) (postgres.BootDataDatabase, error) {
	var (
		err error
		bddb postgres.BootDataDatabase
	)
	ix := uint64(1)

	for ; ix <= retryCount; ix++ {
		log.Printf("Attempting connection to Postgres (attempt %d)", ix)
		bddb, err = postgres.Connect(host, port, user, password, ssl)
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
	var h string
	parseEnv("BSS_ENDPOINT_HOST", &h)
	if h == "" {
		var err error
		h, err = os.Hostname()
		if err == nil {
			if strings.Contains(h, "cray-bss") {
				h = "cray-bss"
			} else {
				h += httpListen
			}
		} else {
			// If all else fails, use localhost
			// This may not work for NFD, but things are kind of
			// messed up anyway if you can't get the hostname.
			log.Printf("Could not get hostname: %s", err)
			h = "localhost" + httpListen
		}
	}
	url := "http://" + h + notifierEndpoint
	log.Printf("Notification endpoint: %s", url)
	return url
}

func main() {
	insecure := false
	spireServiceURL := "https://spire-tokens.spire:54440"

	// Note: Default for --hsm is somewhat irrelevant since it is explicitly
	//       specified in the Dockerfile, and can be overridden via
	//       an environment variable.  Note that the Dockerfile can also be
	//       over-ridden via helm.
	// Note: The Default for --datastore is based on the environment variables
	//       ETCD_HOST and ETCD_PORT, which boot-script-service looks for
	//       explicitly.  See func kvDefaultURL()

	parseEnv("BSS_HTTP_LISTEN", &httpListen)
	parseEnv("HSM_URL", &hsmBase)
	parseEnv("NFD_URL", &nfdBase)
	parseEnv("DATASTORE_BASE", &datastoreBase)
	parseEnv("BSS_INSECURE", &insecure)
	parseEnv("BSS_DEBUG", &debugFlag)
	parseEnv("BSS_RETRY_DELAY", &retryDelay)
	parseEnv("BSS_RETRIEVAL_DELAY", &hsmRetrievalDelay)
	parseEnv("SPIRE_TOKEN_URL", &spireServiceURL)
	parseEnv("BSS_ADVERTISE_ADDRESS", &advertiseAddress)

	flag.StringVar(&httpListen, "http-listen", httpListen, "HTTP server IP + port binding")
	flag.StringVar(&hsmBase, "hsm", hsmBase, "Hardware State Manager location as URI, e.g. [scheme]://[host[:port]]")
	flag.StringVar(&nfdBase, "nfd", nfdBase, "Notification daemon location as URI, e.g. [scheme]://[host[:port]]")
	flag.StringVar(&datastoreBase, "datastore", kvDefaultURL(), "Datastore Service location as URI")
	flag.StringVar(&sqlHost, "postgres-host", sqlHost, "Postgres host as IP address or name")
	flag.StringVar(&serviceName, "service-name", serviceName, "Boot script service name")
	flag.StringVar(&spireTokensBaseURL, "spire-url", spireServiceURL, "Spire join token service base URL")
	flag.StringVar(&advertiseAddress, "cloud-init-address", advertiseAddress, "IP:PORT to advertise for cloud-init calls. This needs to be an IP as we do not have DNS when cloud-init runs")
	flag.BoolVar(&insecure, "insecure", insecure, "Don't enforce https certificate security")
	flag.BoolVar(&sqlInsecure, "postgres-insecure", sqlInsecure, "Don't enforce certificate authority for Postgres")
	flag.BoolVar(&debugFlag, "debug", debugFlag, "Enable debug output")
	flag.BoolVar(&useSQL, "postgres", useSQL, "Use Postgres instead of ETCD")
	flag.UintVar(&retryDelay, "retry-delay", retryDelay, "Retry delay in seconds")
	flag.UintVar(&hsmRetrievalDelay, "hsm-retrieval-delay", hsmRetrievalDelay, "SM Retrieval delay in seconds")
	flag.UintVar(&sqlPort, "postgres-port", sqlPort, "Postgres port")
	flag.Parse()

	sn, snerr := base.GetServiceInstanceName()
	if snerr == nil {
		serviceName = sn
	}
	log.Printf("Service %s started", serviceName)
	initHandlers()

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

	err := SmOpen(hsmBase, svcOpts)
	if err != nil {
		log.Fatalf("Access to SM service %s failed: %v\n", hsmBase, err)
	}

	notifier = newNotifier(serviceName, nfdBase+"/hmi/v1/subscribe", getNotifierURL(), svcOpts)

	// If --postgres passed, use Postgres. Otherwise, use Etcd.
	if useSQL {
		sqlRetryCount, sqlRetryWait, err := sqlDefaultRetryConfig()
		if err != nil {
			log.Println("WARNING: getting retry config failed: ", err)
			log.Printf("WARNING: using default retry config: SQL_RETRY_COUNT=%d SQL_RETRY_WAIT=%d\n", sqlRetryCount, sqlRetryWait)
		}

		var sqlUser, sqlPassword string
		sqlUser, sqlPassword, err = sqlGetCreds()
		if err != nil {
			log.Fatalf("ERROR: could not get Postgres credentials: %v\n", err)
		}

		// Initiate connection to Postgres.
		log.Printf("Using insecure connection to SQL database: %v\n", sqlInsecure)
		bssdb, err = sqlOpen(sqlHost, sqlPort, sqlUser, sqlPassword, !sqlInsecure, sqlRetryCount, sqlRetryWait)
		if err != nil {
			log.Fatalf("Access to Postgres database at %s:%d failed: %v\n", sqlHost, sqlPort, err)
		}
		defer sqlClose()

		// Create database and tables (if they do not exist).
		err = bssdb.CreateDB(bssdbName)
		if err != nil {
			log.Fatalf("Creating Postgres database %q and tables failed: %v\n", bssdbName, err)
		}
	} else {
		kvRetyCount, kvRetryWait, err := kvDefaultRetryConfig()
		if err != nil {
			log.Fatal("Unable to parse ETCD default")
		}

		err = kvOpen(datastoreBase, svcOpts, kvRetyCount, kvRetryWait)
		if err != nil {
			log.Fatalf("Access to Datastore service %s with name %s failed: %v\n", datastoreBase, serviceName, err)
		}
	}
	err = spireTokenServiceInit(spireServiceURL, svcOpts)
	if err != nil {
		// NOTE: Should this be fatal???  Right now, we will continue.
		log.Printf("WARNING: Spire join token service %s access failure: %s", spireServiceURL, err)
	}
	log.Fatal(http.ListenAndServe(httpListen, nil))
}
