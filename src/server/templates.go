package server

import (
	"fmt"
	"net/http"
)

// Common HTML template parts
const htmlHead = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=5.0, user-scalable=yes">
    <meta name="theme-color" content="#4CAF50">
    <title>%s - %s</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: #1a1a1a;
            color: #e0e0e0;
            line-height: 1.6;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 15px 20px;
        }
        header {
            background: #2a2a2a;
            border-bottom: 2px solid #4CAF50;
            padding: 15px 0;
            margin-bottom: 20px;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        header .container {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 15px;
            flex-wrap: wrap;
        }
        .logo {
            font-size: clamp(18px, 4vw, 24px);
            font-weight: bold;
            color: #4CAF50;
            text-decoration: none;
            white-space: nowrap;
        }
        nav {
            display: flex;
            gap: 15px;
            flex-wrap: wrap;
        }
        nav a {
            color: #e0e0e0;
            text-decoration: none;
            padding: 8px 12px;
            border-radius: 5px;
            transition: background 0.3s;
            font-size: clamp(13px, 2.5vw, 15px);
            white-space: nowrap;
        }
        nav a:hover, nav a.active {
            background: #333;
            color: #4CAF50;
        }
        main {
            min-height: 60vh;
        }
        h1 {
            color: #4CAF50;
            margin-bottom: 20px;
            font-size: clamp(24px, 5vw, 32px);
        }
        h2 {
            color: #4CAF50;
            margin-top: 30px;
            margin-bottom: 15px;
            font-size: clamp(20px, 4vw, 24px);
        }
        p {
            margin-bottom: 15px;
            color: #ccc;
        }
        .section {
            background: #2a2a2a;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            border-left: 4px solid #4CAF50;
        }
        footer {
            background: #2a2a2a;
            border-top: 2px solid #4CAF50;
            padding: 25px 0;
            margin-top: 50px;
        }
        footer .container {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 20px;
            flex-wrap: wrap;
        }
        footer p {
            color: #888;
            margin: 0;
            font-size: clamp(12px, 2.5vw, 14px);
        }
        footer a {
            color: #4CAF50;
            text-decoration: none;
        }
        footer a:hover {
            text-decoration: underline;
        }
        footer nav {
            gap: 12px;
        }
        .btn {
            display: inline-block;
            padding: 12px 24px;
            background: #4CAF50;
            color: white;
            text-decoration: none;
            border-radius: 5px;
            border: none;
            cursor: pointer;
            font-size: 16px;
            transition: background 0.3s;
            touch-action: manipulation;
        }
        .btn:hover {
            background: #45a049;
        }
        .search-form {
            display: flex;
            gap: 10px;
            margin: 30px 0;
            flex-wrap: wrap;
        }
        .search-form input[type="text"] {
            flex: 1;
            min-width: 200px;
            padding: 15px;
            font-size: 16px;
            border: 2px solid #333;
            border-radius: 5px;
            background: #2a2a2a;
            color: #e0e0e0;
        }
        ul {
            margin: 15px 0 15px 25px;
            color: #ccc;
        }
        li {
            margin: 8px 0;
        }
        html, body {
            height: 100%%;
            display: flex;
            flex-direction: column;
        }
        body > *:not(footer) {
            flex: 1 0 auto;
        }
        footer {
            flex-shrink: 0;
            margin-top: auto;
        }
        footer .container {
            justify-content: center;
            text-align: center;
        }
        @media (max-width: 768px) {
            header .container {
                padding: 10px 15px;
            }
            .container {
                padding: 10px 15px;
            }
            nav {
                width: 100%%;
                justify-content: flex-start;
                gap: 10px;
            }
            nav a {
                padding: 6px 10px;
            }
            footer .container {
                flex-direction: column;
                align-items: center;
            }
            .section {
                padding: 15px;
            }
        }
        @media (max-width: 480px) {
            .logo {
                font-size: 16px;
            }
            nav a {
                font-size: 13px;
                padding: 5px 8px;
            }
            .btn {
                padding: 10px 18px;
                font-size: 14px;
            }
        }
    </style>
</head>
<body>`

const htmlFooter = `
    <footer>
        <div class="container">
            <p>&copy; 2024 %s. Privacy-Respecting Search.</p>
            <nav>
                <a href="/about">About</a>
                <a href="/privacy">Privacy</a>
                %s
            </nav>
        </div>
    </footer>
</body>
</html>`

func (s *Server) renderHeader(w http.ResponseWriter, title string, activePage string) {
	contactLink := ""
	if s.isContactEnabled() {
		activeContact := ""
		if activePage == "contact" {
			activeContact = " class=\"active\""
		}
		contactLink = fmt.Sprintf("<a href=\"/contact\"%s>Contact</a>", activeContact)
	}

	activeHome := ""
	activeAbout := ""
	activePrivacy := ""

	switch activePage {
	case "home":
		activeHome = " class=\"active\""
	case "about":
		activeAbout = " class=\"active\""
	case "privacy":
		activePrivacy = " class=\"active\""
	}

	fmt.Fprintf(w, htmlHead, title, s.config.Server.Title)
	fmt.Fprintf(w, `
    <header>
        <div class="container">
            <a href="/" class="logo">🔍 %s</a>
            <nav>
                <a href="/"%s>Home</a>
                <a href="/about"%s>About</a>
                <a href="/privacy"%s>Privacy</a>
                %s
            </nav>
        </div>
    </header>
    <main class="container">`,
		s.config.Server.Title,
		activeHome,
		activeAbout,
		activePrivacy,
		contactLink,
	)
}

func (s *Server) renderFooter(w http.ResponseWriter) {
	contactLink := ""
	if s.isContactEnabled() {
		contactLink = "<a href=\"/contact\">Contact</a>"
	}

	fmt.Fprintf(w, htmlFooter, s.config.Server.Title, contactLink)
}
