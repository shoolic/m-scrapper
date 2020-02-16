package margonemscrapper

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jinzhu/gorm"
)

type StatsScrapper struct {
	db        *gorm.DB
	wg        sync.WaitGroup
	timestamp time.Time
}

type GeneralStats struct {
	Timestamp  time.Time
	Online     int
	MaxOnline  int
	Players    int
	Characters int
	Players24h int
}

type WorldStats struct {
	Timestamp       time.Time
	World           string
	TotalCharacters int
	Load1min        int
	Load5min        int
	Online          int
	MaxOnline       int
	Private         bool
}

type CharacterActivity struct {
	Timestamp time.Time
	World     string `gorm:"type:varchar(20);"`
	Nick      string `gorm:"type:varchar(30);"`
}

func (ss *StatsScrapper) Init(db *gorm.DB) {
	ss.db = db
}

func (ss *StatsScrapper) Start(interval time.Duration) {

	ticker := time.NewTicker(interval)

	go func() {
		ss.Scrap()
		for {
			<-ticker.C
			ss.Scrap()
		}
	}()
}

func (ss *StatsScrapper) Scrap() {
	ss.wg.Add(3)
	ss.scrapGeneralStats()
	ss.scrapWorldStats()
	ss.scrapCharacterActivity()
	ss.wg.Wait()
}

func (ss *StatsScrapper) scrapGeneralStats() error {
	fmt.Printf("[ %s ] Processing general stats started...\n", time.Now().Format("2006-01-02 15:04:05"))

	defer ss.wg.Done()

	start := time.Now()
	ss.timestamp = time.Now()

	body, err := ss.getPageBody(STATS_URI)
	if err != nil {
		return err
	}

	doc, err := ss.getPageDocument(body)
	if err != nil {
		return err
	}

	ss.fetchGeneralStats(doc)

	fmt.Printf("[ %s ] Processing general stats finished in %s\n", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).String())
	return nil
}

func (ss *StatsScrapper) scrapWorldStats() error {
	fmt.Printf("[ %s ] Processing worlds stats started...\n", time.Now().Format("2006-01-02 15:04:05"))
	defer ss.wg.Done()
	start := time.Now()
	ss.timestamp = time.Now()

	body, err := ss.getPageBody(WORLDS_STATS_URI)
	if err != nil {
		return err
	}

	doc, err := ss.getPageDocument(body)
	if err != nil {
		return err
	}

	ss.fetchAllPublicWorldStats(doc)
	ss.fetchAllPrivateWorldStats(doc)

	fmt.Printf("[ %s ] Processing worlds stats finished in %s\n", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).String())
	return nil
}

func (ss *StatsScrapper) scrapCharacterActivity() error {
	fmt.Printf("[ %s ] Processing characters activities stats started...\n", time.Now().Format("2006-01-02 15:04:05"))
	defer ss.wg.Done()

	start := time.Now()
	ss.timestamp = time.Now()

	body, err := ss.getPageBody(WORLDS_STATS_URI)
	if err != nil {
		return err
	}

	doc, err := ss.getPageDocument(body)
	if err != nil {
		return err
	}
	ss.fetchAllCharacterActivity(doc)

	fmt.Printf("[ %s ] Processing characters activities stats finished in %s\n", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).String())

	return nil
}

func (ss *StatsScrapper) getPageBody(url string) (io.ReadCloser, error) {
	res, err := http.Get(url)
	if err != nil {
		fmt.Printf("%s", err.Error())
	}
	return res.Body, err
}

func (ss *StatsScrapper) getPageDocument(body io.ReadCloser) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(body)
}

func (ss *StatsScrapper) fetchGeneralStats(doc *goquery.Document) {
	generalStats := &GeneralStats{}
	generalStats.Timestamp = ss.timestamp

	doc.Find(".server-stat-value").Each(func(index int, value *goquery.Selection) {
		text := ss.trim(value.Text())
		switch index {
		case 0:
			generalStats.Online, _ = ss.fetchNumber(text)
		case 1:
			generalStats.MaxOnline, _ = ss.fetchNumber(text)
		case 2:
			generalStats.Players, _ = ss.fetchNumberWithSuffix(text)
		case 3:
			generalStats.Characters, _ = ss.fetchNumberWithSuffix(text)
		case 4:
			generalStats.Players24h, _ = ss.fetchNumber(text)
		}
	})

	ss.db.Create(generalStats)
}

func (ss *StatsScrapper) fetchAllCharacterActivity(doc *goquery.Document) {
	worldNameRegexp, _ := regexp.Compile("^.+:")
	doc.Find(".current-players-overlay").Children().Each(func(index int, worldHTML *goquery.Selection) {
		if index != 0 {
			worldName := worldNameRegexp.FindString(ss.trim(worldHTML.Find(".news-header").First().Text()))
			worldName = worldName[:len(worldName)-1]
			nicksText := strings.Replace(worldHTML.Find(".news-body").First().Text(), "Legenda                        >>", "", 1)
			nicks := strings.Split(ss.trim(nicksText), ", ")
			for _, nick := range nicks {
				characterActivity := &CharacterActivity{}
				characterActivity.Timestamp = ss.timestamp
				characterActivity.World = worldName
				characterActivity.Nick = nick
				ss.db.Create(characterActivity)
			}
		}
	})
}

func (ss *StatsScrapper) fetchAllPublicWorldStats(doc *goquery.Document) {
	doc.Find(".public-world-list > .server-stats").Each(func(index int, worldStatsHTML *goquery.Selection) {
		worldStats := ss.fetchWorldStats(worldStatsHTML)
		ss.db.Create(worldStats)
	})
}

func (ss *StatsScrapper) fetchAllPrivateWorldStats(doc *goquery.Document) {
	doc.Find(".private-world-list > .server-stats").Each(func(index int, worldStatsHTML *goquery.Selection) {
		worldStats := ss.fetchPrivateWorldStats(worldStatsHTML)
		ss.db.Create(worldStats)
	})
}

func (ss *StatsScrapper) fetchWorldStats(worldStatsHTML *goquery.Selection) *WorldStats {
	worldStats := &WorldStats{}

	worldName, _ := worldStatsHTML.Attr("data-name")
	worldName = strings.Title(worldName[1:])
	worldStats.World = worldName
	worldStats.Timestamp = ss.timestamp
	worldStatsHTML.Find("td").Each(func(index int, cell *goquery.Selection) {
		text := ss.trim(cell.Text())
		switch index {
		case 1:
			worldStats.TotalCharacters, _ = ss.fetchNumberWithSuffix(text)
		case 3:
			worldStats.Load1min, _ = ss.fetchNumberWithPercent(text)
		case 5:
			worldStats.Load5min, _ = ss.fetchNumberWithPercent(text)
		case 7:
			worldStats.Online, _ = ss.fetchNumber(text)
		case 9:
			worldStats.MaxOnline, _ = ss.fetchNumber(text)
		}
	})
	return worldStats
}

func (ss *StatsScrapper) fetchPrivateWorldStats(worldStatsHTML *goquery.Selection) *WorldStats {
	worldStats := ss.fetchWorldStats(worldStatsHTML)
	worldStats.Private = true
	return worldStats
}

func (ss *StatsScrapper) fetchNumberWithSuffix(text string) (int, error) {
	if res, _ := regexp.MatchString("tys.", text); res {
		text = text[:len(text)-5]
		num, err := strconv.ParseFloat(text, 32)
		if err == nil {
			num *= 1000
			return int(num), nil
		}
	}
	num, err := strconv.ParseInt(text, 10, 32)

	if err == nil {
		return int(num), nil
	}
	return 0, err
}

func (ss *StatsScrapper) fetchNumberWithPercent(text string) (int, error) {
	text = strings.TrimRight(text, "%")
	online, err := strconv.ParseInt(text, 10, 32)
	if err == nil {
		return int(online), nil
	}
	return 0, err
}

func (ss *StatsScrapper) fetchNumber(text string) (int, error) {
	text = strings.ReplaceAll(text, " ", "")
	online, err := strconv.ParseInt(text, 10, 32)
	if err == nil {
		return int(online), nil
	}
	return 0, err
}

func (ss *StatsScrapper) trim(str string) string {
	return strings.Trim(str, "\n\t ")
}
