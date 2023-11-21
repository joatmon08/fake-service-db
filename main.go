package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	hclog "github.com/hashicorp/go-hclog"
	_ "github.com/lib/pq"

	"github.com/nicholasjackson/env"
)

const timeFormat = "2006-01-02T15:04:05.000000"

var db *sql.DB

var listenAddress = env.String("LISTEN_ADDR", false, "0.0.0.0:9090", "IP address and port to bind service to")

var name = env.String("NAME", false, "Service", "Name of the service")

var databaseHost = env.String("DATABASE_HOST", false, "127.0.0.1", "Host for PostgreSQL database")
var databasePort = env.Int("DATABASE_PORT", false, 5432, "Port of PostgreSQL database")
var databaseUser = env.String("DATABASE_USER", false, "", "Username for PostgreSQL database")
var databasePassword = env.String("DATABASE_PASSWORD", false, "", "Password for PostgreSQL database")
var databaseName = env.String("DATABASE_NAME", false, "", "Name of database for PostgreSQL instance")

type App struct {
	Router *mux.Router
	DB     *sql.DB
}

func (a *App) Initialize(user, password, host, dbname string, port int) {
	hclog.Default().Info("Attempting to connect to database", "host", host, "user", user)
	connectionString :=
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, dbname)

	var err error
	a.DB, err = sql.Open("postgres", connectionString)
	if err != nil {
		hclog.Default().Error("Cannot connect to database, %s", err)
	}

	a.Router = mux.NewRouter()
}

func (a *App) Run(addr string) {
	http.ListenAndServe(*listenAddress, a.Router)
}

func main() {
	env.Parse()

	a := App{}

	hclog.Default().Info("Starting fake-service-db")

	a.Initialize(*databaseUser, *databasePassword, *databaseHost, *databaseName, *databasePort)

	a.Router = mux.NewRouter()

	a.initializeRoutes()

	a.Run(*listenAddress)
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/", a.getCustomers).Methods("GET")
	a.Router.HandleFunc("/health", a.health).Methods("GET")
	a.Router.HandleFunc("/ready", a.health).Methods("GET")
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, "OK")
}

func (a *App) getCustomers(w http.ResponseWriter, r *http.Request) {
	resp := &Response{
		Name: *name,
	}

	ts := time.Now()
	te := time.Now()
	resp.StartTime = ts.Format(timeFormat)
	resp.EndTime = te.Format(timeFormat)
	resp.Duration = te.Sub(ts).String()

	customers, err := getCustomers(a.DB)
	if err != nil {
		resp.Body = json.RawMessage(fmt.Sprintf(`"%s"`, err.Error()))
		resp.Code = http.StatusInternalServerError
		respondWithJSON(w, http.StatusInternalServerError, resp)
		return
	}

	message := fmt.Sprintf("Hello %s", strings.Join(customers, " "))

	resp.Body = json.RawMessage(fmt.Sprintf(`"%s"`, message))
	resp.Code = http.StatusOK
	respondWithJSON(w, http.StatusOK, resp)
}

type customer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func getCustomers(db *sql.DB) ([]string, error) {
	hclog.Default().Info("Getting customers from database")
	rows, err := db.Query(
		"SELECT name FROM customers")

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	customers := []string{}

	for rows.Next() {
		var c customer
		if err := rows.Scan(&c.Name); err != nil {
			return nil, err
		}
		customers = append(customers, c.Name)
	}

	return customers, nil
}

// Response defines the type which is returned from the service
type Response struct {
	Name          string              `json:"name,omitempty"`
	URI           string              `json:"uri,omitempty"` // Called URI by downstream
	Type          string              `json:"type,omitempty"`
	IPAddresses   []string            `json:"ip_addresses,omitempty"`
	Path          []string            `json:"path,omitempty"` // Path received by upstream
	StartTime     string              `json:"start_time,omitempty"`
	EndTime       string              `json:"end_time,omitempty"`
	Duration      string              `json:"duration,omitempty"`
	Headers       map[string]string   `json:"headers,omitempty"`
	Cookies       map[string]string   `json:"cookies,omitempty"`
	Body          json.RawMessage     `json:"body,omitempty"`
	UpstreamCalls map[string]Response `json:"upstream_calls,omitempty"`
	Code          int                 `json:"code"`
	Error         string              `json:"error,omitempty"`
}

// ToJSON converts the response to a JSON string
func (r *Response) ToJSON() string {
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(r)
	if err != nil {
		panic(err)
	}

	return buffer.String()
}

// FromJSON populates the response from a JSON string
func (r *Response) FromJSON(d []byte) error {
	resp := &Response{}
	err := json.Unmarshal(d, resp)
	if err != nil {
		return err
	}

	*r = *resp

	return nil
}
