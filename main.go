package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"
)

type Bookmark struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
}

type NetscapeBookmark struct {
	XMLName xml.Name `xml:"NETSCAPE-Bookmark-file-1"`
	DOCTYPE string   `xml:",innerxml"`
	META    struct {
		HTTPEquiv string `xml:"http-equiv,attr"`
		Content   string `xml:"content,attr"`
	} `xml:"META"`
	TITLE string `xml:"TITLE"`
	H1    string `xml:"H1"`
	DL    struct {
		DT []struct {
			H3 string `xml:"H3,omitempty"`
			A  struct {
				HREF    string `xml:"HREF,attr"`
				AddDate string `xml:"ADD_DATE,attr"`
			} `xml:"A"`
		} `xml:"DT"`
	} `xml:"DL"`
}

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./bookmarks.db")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS bookmarks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL UNIQUE,
		thumbnail TEXT NOT NULL
	)`)
	if err != nil {
		log.Fatal(err)
	}
}

func captureScreenshot(url string) ([]byte, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.CaptureScreenshot(&buf),
	)

	return buf, err
}

func addBookmark(c *gin.Context) {
	url := c.PostForm("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	screenshot, err := captureScreenshot(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to capture screenshot"})
		return
	}

	result, err := db.Exec("INSERT INTO bookmarks (url, thumbnail) VALUES (?, ?)", url, "")
	if err != nil {
		if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			c.JSON(http.StatusConflict, gin.H{"error": "This URL is already bookmarked"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add bookmark"})
		}
		return
	}

	id, _ := result.LastInsertId()
	thumbnailPath := filepath.Join("thumbnails", fmt.Sprintf("%d.png", id))
	if err := os.WriteFile(thumbnailPath, screenshot, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save thumbnail"})
		return
	}

	_, err = db.Exec("UPDATE bookmarks SET thumbnail = ? WHERE id = ?", thumbnailPath, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bookmark"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Bookmark added successfully"})
}

func getBookmarks() ([]Bookmark, error) {
	rows, err := db.Query("SELECT id, url, thumbnail FROM bookmarks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.URL, &b.Thumbnail); err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, b)
	}

	return bookmarks, nil
}

func deleteBookmark(c *gin.Context) {
	id := c.Param("id")
	_, err := db.Exec("DELETE FROM bookmarks WHERE id = ?", id)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to delete bookmark"})
		return
	}

	c.Redirect(http.StatusSeeOther, "/")
}

func importNetscapeBookmarks(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": "Failed to get file"})
		return
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to read file"})
		return
	}

	var netscapeBookmark NetscapeBookmark
	if err := xml.Unmarshal(bytes, &netscapeBookmark); err != nil {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": "Failed to parse Netscape bookmark file"})
		return
	}

	importedCount := 0
	for _, dt := range netscapeBookmark.DL.DT {
		url := dt.A.HREF

		screenshot, err := captureScreenshot(url)
		if err != nil {
			log.Printf("Failed to capture screenshot for %s: %v", url, err)
			continue
		}

		result, err := db.Exec("INSERT INTO bookmarks (url, thumbnail) VALUES (?, ?)", url, "")
		if err != nil {
			if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
				log.Printf("URL already exists: %s", url)
			} else {
				log.Printf("Failed to add bookmark for %s: %v", url, err)
			}
			continue
		}

		id, _ := result.LastInsertId()
		thumbnailPath := filepath.Join("thumbnails", fmt.Sprintf("%d.png", id))
		if err := os.WriteFile(thumbnailPath, screenshot, 0644); err != nil {
			log.Printf("Failed to save thumbnail for %s: %v", url, err)
			continue
		}

		_, err = db.Exec("UPDATE bookmarks SET thumbnail = ? WHERE id = ?", thumbnailPath, id)
		if err != nil {
			log.Printf("Failed to update bookmark for %s: %v", url, err)
			continue
		}

		importedCount++
	}

	c.HTML(http.StatusOK, "index.html", gin.H{"message": fmt.Sprintf("Successfully imported %d new bookmarks", importedCount)})
}

func exportBookmarks(c *gin.Context) {
	bookmarks, err := getBookmarks()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to get bookmarks"})
		return
	}

	netscapeBookmark := NetscapeBookmark{
		DOCTYPE: `DOCTYPE NETSCAPE-Bookmark-file-1`,
		META: struct {
			HTTPEquiv string `xml:"http-equiv,attr"`
			Content   string `xml:"content,attr"`
		}{
			HTTPEquiv: "Content-Type",
			Content:   "text/html; charset=UTF-8",
		},
		TITLE: "Bookmarks",
		H1:    "Bookmarks",
	}

	for _, bookmark := range bookmarks {
		netscapeBookmark.DL.DT = append(netscapeBookmark.DL.DT, struct {
			H3 string `xml:"H3,omitempty"`
			A  struct {
				HREF    string `xml:"HREF,attr"`
				AddDate string `xml:"ADD_DATE,attr"`
			} `xml:"A"`
		}{
			A: struct {
				HREF    string `xml:"HREF,attr"`
				AddDate string `xml:"ADD_DATE,attr"`
			}{
				HREF:    bookmark.URL,
				AddDate: fmt.Sprintf("%d", time.Now().Unix()),
			},
		})
	}

	output, err := xml.MarshalIndent(netscapeBookmark, "", "    ")
	if err != nil {
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to generate export file"})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=bookmarks.html")
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, "<!DOCTYPE NETSCAPE-Bookmark-file-1>\n"+string(output))
}

func indexHandler(c *gin.Context) {
	bookmarks, err := getBookmarks()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to get bookmarks"})
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{"bookmarks": bookmarks})
}

func main() {
	initDB()
	defer db.Close()

	if err := os.MkdirAll("thumbnails", 0755); err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	})
	r.LoadHTMLGlob("templates/*")
	r.Static("/thumbnails", "./thumbnails")

	r.GET("/", indexHandler)
	r.POST("/bookmarks", addBookmark)
	r.POST("/bookmarks/:id/delete", deleteBookmark)
	r.POST("/import", importNetscapeBookmarks)
	r.GET("/export", exportBookmarks)

	r.Run(":8080")
}
