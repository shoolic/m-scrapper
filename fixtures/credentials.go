package fixtures

type Credentials struct {
	Postgres PostgresCredentials
}

type PostgresCredentials struct {
	Host     string
	Port     int
	User     string
	Dbname   string
	Password string
}
