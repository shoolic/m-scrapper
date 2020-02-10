package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/jinzhu/gorm"
	"github.com/shoolic/m-scrapper/fixtures"
)

func GetCredentials() (*fixtures.Credentials, error) {
	file, _ := ioutil.ReadFile("credentials.json")
	credentials := &fixtures.Credentials{}
	_ = json.Unmarshal([]byte(file), credentials)

	return credentials, nil
}

func OpenPostgres(postgresCredentials *fixtures.PostgresCredentials) (*gorm.DB, error) {
	postgresSettings := fmt.Sprintf("host=%s port=%d user=%s dbname=%s password=%s",
		postgresCredentials.Host,
		postgresCredentials.Port,
		postgresCredentials.User,
		postgresCredentials.Dbname,
		postgresCredentials.Password)

	db, err := gorm.Open("postgres", postgresSettings)
	if err != nil {
		panic(err.Error())
	}

	return db, nil
}
