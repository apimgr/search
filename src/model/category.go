package model

import "strings"

// Category represents a search category
type Category string

const (
	CategoryGeneral Category = "general"
	CategoryImages  Category = "images"
	CategoryVideos  Category = "videos"
	CategoryNews    Category = "news"
	CategoryMaps    Category = "maps"
	CategoryFiles   Category = "files"
	CategoryMusic   Category = "music"
	CategoryIT      Category = "it"
	CategoryScience Category = "science"
	CategorySocial  Category = "social"
)

// AllCategories returns all available categories
func AllCategories() []Category {
	return []Category{
		CategoryGeneral,
		CategoryImages,
		CategoryVideos,
		CategoryNews,
		CategoryMaps,
		CategoryFiles,
		CategoryMusic,
		CategoryIT,
		CategoryScience,
		CategorySocial,
	}
}

// ParseCategory normalizes a category string to a supported category.
// It accepts legacy aliases used elsewhere in the codebase.
func ParseCategory(value string) Category {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "general", "web":
		return CategoryGeneral
	case "images":
		return CategoryImages
	case "videos":
		return CategoryVideos
	case "news":
		return CategoryNews
	case "maps":
		return CategoryMaps
	case "files":
		return CategoryFiles
	case "music":
		return CategoryMusic
	case "it", "code":
		return CategoryIT
	case "science":
		return CategoryScience
	case "social":
		return CategorySocial
	default:
		return CategoryGeneral
	}
}

// String returns the string representation of a category
func (c Category) String() string {
	return string(c)
}

// IsValid checks if the category is valid
func (c Category) IsValid() bool {
	for _, cat := range AllCategories() {
		if cat == c {
			return true
		}
	}
	return false
}
