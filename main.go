package main

import (
	"time"

	_ "github.com/jinzhu/gorm/dialects/postgres"

	"github.com/shoolic/m-scrapper/margonemscrapper"
	"github.com/shoolic/m-scrapper/utils"
)

func main() {

	credentials, _ := utils.GetCredentials()
	db, _ := utils.OpenPostgres(&credentials.Postgres)
	defer db.Close()

	db.AutoMigrate(&margonemscrapper.FullCharacter{},
		&margonemscrapper.CharacterLevel{},
		&margonemscrapper.GeneralStats{},
		&margonemscrapper.WorldStats{},
		&margonemscrapper.CharacterActivity{})

	done := make(chan struct{}, 1)

	telawelScrapper := &margonemscrapper.WorldLadderScrapper{}
	telawelScrapper.Init(db, "Telawel", margonemscrapper.FullScrap)
	telawelScrapper.Start(1 * time.Hour)

	<-done
}
