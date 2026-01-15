package model

// Category represents a search category
type Category string

const (
	CategoryGeneral Category = "general"
	CategoryImages  Category = "images"
	CategoryVideos  Category = "videos"
	CategoryNews    Category = "news"
	CategoryMaps    Category = "maps"
	CategoryFiles   Category = "files"
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
		CategoryIT,
		CategoryScience,
		CategorySocial,
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
