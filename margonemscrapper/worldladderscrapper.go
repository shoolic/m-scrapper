package margonemscrapper

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jinzhu/gorm"
)

type ScrapMode int

const (
	FullScrap  ScrapMode = 0
	LevelScrap ScrapMode = 1
)

var classes = map[string]string{
	"Tropiciel":      "t",
	"Łowca":          "h",
	"Tancerz ostrzy": "b",
	"Wojownik":       "w",
	"Paladyn":        "p",
	"Mag":            "m",
}

type WorldLadderScrapper struct {
	db         *gorm.DB
	world      string
	processRow processRowFn
	wg         sync.WaitGroup
	pageBodies chan io.ReadCloser
	pageDocs   chan *goquery.Document
	timestamp  time.Time
}

type Document struct {
	index int
	doc   *goquery.Document
}

type FullCharacter struct {
	Timestamp   time.Time
	World       string `gorm:"type:varchar(20);primary_key;auto_increment:false"`
	ProfileID   int    // `gorm:"primary_key;auto_increment:false"`
	CharacterID int    `gorm:"primary_key;auto_increment:false"`
	Nick        string `gorm:"type:varchar(20);"`
	Level       int
	Class       string `gorm:"type:char(1)"`
	Honor       int
	Last        string `gorm:"type:varchar(30);"`
}

type CharacterLevel struct {
	id          int64 `gorm:"primary_key;"`
	Timestamp   time.Time
	World       string `gorm:"type:varchar(20);"`
	CharacterID int
}

type processRowFn func(int, *goquery.Selection)

func New() *WorldLadderScrapper {
	return &WorldLadderScrapper{}
}

func (wls *WorldLadderScrapper) Init(db *gorm.DB, world string, mode ScrapMode) {
	wls.db = db
	wls.world = world

	switch mode {
	case FullScrap:
		wls.processRow = wls.scrapFull
	case LevelScrap:
		wls.processRow = wls.scrapLevel
	}
}

func (wls *WorldLadderScrapper) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		wls.Scrap()
		for {
			<-ticker.C
			wls.Scrap()
		}
	}()
}

func (wls *WorldLadderScrapper) Scrap() {
	fmt.Printf("[ %s ] Downloading ladder of %s world...\n", time.Now().Format("2006-01-02 15:04:05"), wls.world)
	
	wls.timestamp = time.Now()
	start := time.Now()
	
	wls.wg.Add(1)
	
	wls.initChannels()
	
	totalPages, _ := wls.getTotalPages()
	
	wls.wg.Add(totalPages)
	wls.wg.Done()
	
	go wls.getPageBodies(totalPages)
	go wls.getPageDocuments()
	go wls.scrapDocuments()
	
	wls.wg.Wait()

	fmt.Printf("[ %s ] Downloading ladder of %s world finished in %s\n", time.Now().Format("2006-01-02 15:04:05"), wls.world, time.Since(start).String())
}

func (wls *WorldLadderScrapper) initChannels() {
	wls.pageBodies = make(chan io.ReadCloser)
	wls.pageDocs = make(chan *goquery.Document)
}

func (wls *WorldLadderScrapper) getTotalPages() (int, error) {
	firstPageBody, _ := wls.getPageBody(1)
	firstPageDoc, _ := wls.getPageDocument(firstPageBody)
	totalPages, _ := wls.getTotalPagesFromDoc(firstPageDoc)
	return totalPages, nil
}

func (wls *WorldLadderScrapper) getPageBodies(totalPages int) {
	for i := 1; i <= totalPages; i++ {
		fmt.Printf("[ %s ] Downloading page %d of %s world started...\n", time.Now().Format("2006-01-02 15:04:05"), i, wls.world)
		start := time.Now()
		pageBody, err := wls.getPageBody(i)
		if err != nil {
			i--
			continue
		}
		wls.pageBodies <- pageBody
		fmt.Printf("[ %s ] Downloading page %d of %s world finished in %s\n", time.Now().Format("2006-01-02 15:04:05"), i, wls.world, time.Since(start).String())
	}

	close(wls.pageBodies)
}

func (wls *WorldLadderScrapper) getPageDocuments() {
	for pageBody := range wls.pageBodies {
		pageDoc, err := wls.getPageDocument(pageBody)
		if err != nil {
			fullerr := fmt.Errorf("[ %s ] Error in parsing document during %s world ladder scrapping: %w", time.Now().Format("2006-01-02 15:04:05"), wls.world, err)
			fmt.Println(fullerr)
			continue
		}
		wls.pageDocs <- pageDoc
	}
	close(wls.pageDocs)
}

func (wls *WorldLadderScrapper) scrapDocuments() {
	for pageDoc := range wls.pageDocs {
		wls.scrapDocument(pageDoc)
	}
}

func (wls *WorldLadderScrapper) scrapDocument(doc *goquery.Document) {
	doc.Find("tbody").Each(func(index int, table *goquery.Selection) {
		table.Find("tr").Each(wls.processRow)
		table.Find("tr").Each(wls.scrapLevel)
	})
	wls.wg.Done()
}

func (wls *WorldLadderScrapper) getTotalPagesFromDoc(doc *goquery.Document) (int, error) {
	text := strings.Trim(doc.Find(".total-pages").First().Text(), "\n\t ")
	totalPages, err := strconv.ParseInt(text, 10, 32)

	if err == nil {
		return int(totalPages), nil
	}

	return 0, nil
}

func (wls *WorldLadderScrapper) getPageBody(page int) (io.ReadCloser, error) {
	res, err := http.Get(WORLD_LADDER_URI + wls.world + "?page=" + strconv.Itoa(page))
	if err != nil {
		fullerr := fmt.Errorf("[ %s ] Error in downloading page %d during %s world ladder scrapping: %w", time.Now().Format("2006-01-02 15:04:05"), page, wls.world, err)
		fmt.Println(fullerr)
		return nil, err
	}

	return res.Body, nil
}

func (wls *WorldLadderScrapper) getPageDocument(body io.ReadCloser) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(body)
}

func (wls *WorldLadderScrapper) scrapFull(index int, row *goquery.Selection) {
	fullCharacter, _ := wls.fetchFullCharacter(row)
	wls.db.Set("gorm:insert_option", "ON CONFLICT (world, character_id) DO UPDATE SET profile_id = EXCLUDED.profile_id").Create(fullCharacter)
}

func (wls *WorldLadderScrapper) scrapLevel(index int, row *goquery.Selection) {
	characterLevel, _ := wls.fetchCharacterLevel(row)
	wls.db.Create(characterLevel)
}

func (wls *WorldLadderScrapper) fetchFullCharacter(row *goquery.Selection) (*FullCharacter, error) {
	fullCharacter := &FullCharacter{}
	fullCharacter.Timestamp = wls.timestamp
	fullCharacter.World = wls.world
	fullCharacter.ProfileID, fullCharacter.CharacterID, _ = wls.fetchIDs(row)
	fullCharacter.Nick, _ = wls.fetchNick(row)
	fullCharacter.Level, _ = wls.fetchLevel(row)
	fullCharacter.Honor, _ = wls.fetchHonor(row)
	fullCharacter.Class, _ = wls.fetchClass(row)
	fullCharacter.Last, _ = wls.fetchLast(row)

	return fullCharacter, nil
}

func (wls *WorldLadderScrapper) fetchCharacterLevel(row *goquery.Selection) (*CharacterLevel, error) {
	characterLevel := &CharacterLevel{}
	characterLevel.Timestamp = wls.timestamp
	characterLevel.World = wls.world
	_, characterLevel.CharacterID, _ = wls.fetchIDs(row)
	return characterLevel, nil
}

func (wls *WorldLadderScrapper) fetchLevel(row *goquery.Selection) (int, error) {
	text := strings.Trim(row.Find(".long-level").First().Text(), "\n\t ")
	lvl, err := strconv.ParseInt(text, 10, 32)

	if err == nil {
		return int(lvl), nil
	}

	return 0, errors.New("failed to fetch level")
}

func (wls *WorldLadderScrapper) fetchHonor(row *goquery.Selection) (int, error) {
	text := strings.Trim(row.Find(".long-ph").First().Text(), "\n\t ")
	honor, err := strconv.ParseInt(text, 10, 32)

	if err == nil {
		return int(honor), nil
	}

	return 0, errors.New("failed to fetch honor")
}

func (wls *WorldLadderScrapper) fetchLast(row *goquery.Selection) (string, error) {
	return strings.Trim(row.Find(".long-last-online").First().Text(), "\n\t "), nil
}

func (wls *WorldLadderScrapper) fetchClass(row *goquery.Selection) (string, error) {
	return classes[strings.Trim(row.Find(".long-players").First().Text(), "\n\t ")], nil
}

func (wls *WorldLadderScrapper) fetchNick(row *goquery.Selection) (string, error) {
	return strings.Trim(row.Find(".long-clan").First().Text(), "\n\t "), nil
}

func (wls *WorldLadderScrapper) fetchIDs(row *goquery.Selection) (int, int, error) {
	link, exists := row.Find("a").First().Attr("href")
	if exists {
		var profileID, characterID int
		n, err := fmt.Sscanf(link, "/profile/view,%d#char_%d", &profileID, &characterID)
		if err == nil && n == 2 {
			return profileID, characterID, nil
		}
	}

	return 0, 0, errors.New("failed to fetch profile and character IDs")
}
