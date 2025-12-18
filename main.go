package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/mmcdole/gofeed"
	_ "modernc.org/sqlite"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	store, err := NewStorage(cfg.DatabaseFilePath)
	if err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer store.Close()

	if cfg.GenerateAll {
		runGenerateAll(cfg, store)
		return
	}

	llm := NewLLMClient(cfg)

	var writer Writer
	if cfg.TerminalMode {
		writer = TerminalWriter{}
	} else {
		mw := NewMarkdownWriter(cfg, time.Now().Format("2006-01-02"))
		os.Remove(mw.FilePath)
		writer = mw
	}

	fp := gofeed.NewParser()

	DisplayWeather(cfg, writer)
	DisplaySunriseSunset(cfg, writer)

	RunAnalyst(cfg, store, llm, writer, fp)

	for _, feedConfig := range cfg.Feeds {
		ProcessFeed(feedConfig, cfg, store, llm, writer, fp)
	}

}

func runGenerateAll(cfg *Config, store *Storage) {
	fmt.Println("🍵 Regenerating all daily digests from database...")

	allData, err := store.GetAllArticles()
	if err != nil {
		log.Fatalf("Error querying database: %v", err)
	}

	var dates []string
	for d := range allData {
		dates = append(dates, d)
	}
	sort.Strings(dates) // 2023-01-01, 2023-01-02...

	for _, date := range dates {
		items := allData[date]
		fmt.Printf("Processing %s (%d articles)... \n", date, len(items))

		mw := NewMarkdownWriter(cfg, date)
		os.Remove(mw.FilePath)

		// Group items by Feed Title so we can create sections
		feeds := make(map[string][]ArchivedItem)
		var feedOrder []string // To keep consistent order

		for _, item := range items {
			if _, exists := feeds[item.FeedTitle]; !exists {
				feedOrder = append(feedOrder, item.FeedTitle)
			}
			feeds[item.FeedTitle] = append(feeds[item.FeedTitle], item)
		}
		sort.Strings(feedOrder)

		for _, feedTitle := range feedOrder {
			feedItems := feeds[feedTitle]

			// Write Feed Header (using the first item's feed URL for favicon)
			firstItem := feedItems[0]
			mw.Write(mw.WriteFeedHeaderRaw(feedTitle, firstItem.FeedURL))

			for _, item := range feedItems {
				var line string

				// Instapaper Icon (if enabled)
				if cfg.Instapaper {
					instapaperURL := fmt.Sprintf("https://www.instapaper.com/hello2?url=%s", item.URL)
					line += fmt.Sprintf(`[<img height="16" src="https://staticinstapaper.s3.dualstack.us-west-2.amazonaws.com/img/favicon.png">](%s)`, instapaperURL)
				}

				// Article Title Link
				line += fmt.Sprintf("[%s](%s)  \n", item.Title, item.URL)
				mw.Write(line)

				// Summary (if exists)
				if item.Summary != "" {
					mw.Write(mw.WriteSummary(item.Summary, true))
				}
			}
		}
	}
	fmt.Println("Done.")
}
