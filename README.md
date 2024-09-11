# Bookmark Manager

Bookmark Manager is a web application built with Go that allows users to manage their bookmarks. It provides features such as adding bookmarks, capturing screenshots of bookmarked pages, and importing/exporting bookmarks in Netscape format.

## Features

- Add bookmarks with automatic screenshot capture
- View bookmarks with thumbnails
- Delete bookmarks
- Import bookmarks from Netscape format HTML files
- Export bookmarks to Netscape format HTML files
- Prevent duplicate URL entries

## Prerequisites

Before you begin, ensure you have met the following requirements:

- SQLite installed on your system
- Chrome or Chromium installed (for screenshot capture)

## Usage

1. Build the application:
   ```
   $ go build
   ```

2. Run the application:
   ```
   ./bookmark-manager
   ```

3. Open your web browser and navigate to `http://localhost:8080`

## License

This project is licensed under the MIT License - see the [LICENSE](https://opensource.org/license/mit) for details.
