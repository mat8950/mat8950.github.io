# Bookmark Manager

A modern, browser-style bookmark manager with automatic favicon loading and hierarchical folder navigation.

## Features

- 📁 Hierarchical folder structure
- 🔍 Real-time search functionality
- 🌓 Dark/Light theme toggle
- 🎨 Modern, responsive design
- 🖼️ Automatic favicon loading using Google's service
- 📱 Mobile-friendly interface

## Technologies

- Pure HTML, CSS, and JavaScript
- No external dependencies
- Parses Netscape Bookmark format
- Google Favicon API for icons

## Usage

Simply open `index.html` in your browser. The application will automatically load and parse bookmarks from `bookmarks.html`.

## File Structure

- `index.html` - Main application page
- `script.js` - Application logic and bookmark parsing
- `css/` - Styling and themes (linked directly from `index.html`)
- `bookmarks.html` - Bookmark data in Netscape format

## Adding Bookmarks

Edit the `bookmarks.html` file following the Netscape Bookmark format:

```html
<DT><H3>Folder Name</H3>
<DL><p>
    <DT><A HREF="https://example.com">Site Name</A>
</DL><p>
```

The application will automatically fetch favicons for each bookmark.
