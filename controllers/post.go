package controllers

import (
	"net/http"
	"strings"
	"time"

	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/steffen25/golang.zone/app"
	"github.com/steffen25/golang.zone/models"
	"github.com/steffen25/golang.zone/repositories"
	"github.com/steffen25/golang.zone/services"
	"log"
	"strconv"
)

type PostController struct {
	*app.App
	repositories.PostRepository
}

type PostPaginator struct {
	Total        int         `json:"total"`
	PerPage      int         `json:"perPage"`
	CurrentPage  int         `json:"currentPage"`
	LastPage     int         `json:"lastPage"`
	From         int         `json:"from"`
	To           int         `json:"to"`
	FirstPageUrl string      `json:"firstPageUrl"`
	LastPageUrl  string      `json:"lastPageUrl"`
	NextPageUrl  *string     `json:"nextPageUrl"`
	PrevPageUrl  *string     `json:"prevPageUrl"`
	Data         interface{} `json:"data"`
}

func NewPostController(a *app.App, pr repositories.PostRepository) *PostController {
	return &PostController{a, pr}
}

func (pc *PostController) GetAll(w http.ResponseWriter, r *http.Request) {
	total, _ := pc.PostRepository.GetTotalPostCount()
	page := r.URL.Query().Get("page")
	log.Println("page: ", page)
	pageInt, err := strconv.Atoi(page)
	if err != nil {
		pageInt = 1
	}
	perPage := r.URL.Query().Get("perpage")
	log.Println("perpage: ", perPage)
	perPageInt, err := strconv.Atoi(perPage)
	if err != nil || perPageInt < 1 || perPageInt > 100 {
		perPageInt = 10
	}
	offset := (pageInt - 1) * perPageInt
	log.Println("offset: ", offset)
	to := pageInt * perPageInt
	from := offset + 1
	totalPages := (total-1)/perPageInt + 1
	//totalCount := int(float64(total) / float64(perPageInt))
	// $prevPage = ($page + $totalPages - 2) % $totalPages + 1;
	//nextPage := pageInt%totalPages + 1
	//prevPage := (pageInt+totalPages-2)%totalPages + 1
	// $previous_offset = (($currentPage - 1) - 1) * $items_per_page;
	// (currentPage-1 // Should be greater or equal to 0) * limit
	prevPage := pageInt - 1
	log.Println("prevPage: ", prevPage)
	firstPageUrl := fmt.Sprintf(r.URL.Scheme+r.Host+r.URL.Path+"?page=%d", 1)
	lastPageString := fmt.Sprintf(r.URL.Scheme+r.Host+r.URL.Path+"?page=%d", totalPages)
	var prevPageUrl string
	var nextPageUrl string
	if prevPage > 0 && prevPage < totalPages {
		prevPageUrl = fmt.Sprintf(r.URL.Scheme+r.Host+r.URL.Path+"?page=%d", prevPage)
	}
	nextPage := pageInt + 1
	if nextPage <= totalPages {
		nextPageUrl = fmt.Sprintf(r.URL.Scheme+r.Host+r.URL.Path+"?page=%d", nextPage)
	}
	//prevPageString := fmt.Sprintf(r.URL.Path+"?page=%d", prevPage)
	posts, err := pc.PostRepository.Paginate(perPageInt, offset)
	if err != nil {
		NewAPIError(&APIError{false, "Could not fetch posts", http.StatusBadRequest}, w)
		return
	}

	if len(posts) == 0 {
		NewAPIResponse(&APIResponse{Success: false, Message: "Could not find posts", Data: posts}, w, http.StatusNotFound)
		return
	}

	postPaginator := PostPaginator{
		total,
		perPageInt,
		pageInt,
		totalPages,
		from,
		to,
		firstPageUrl,
		lastPageString,
		&nextPageUrl,
		&prevPageUrl,
		posts,
	}

	NewAPIResponse(&APIResponse{Success: true, Data: postPaginator}, w, http.StatusOK)
}

func (pc *PostController) GetById(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		NewAPIError(&APIError{false, "Invalid request", http.StatusBadRequest}, w)
		return
	}
	post, err := pc.PostRepository.FindById(id)
	if err != nil {
		NewAPIError(&APIError{false, "Could not find post", http.StatusNotFound}, w)
		return
	}

	NewAPIResponse(&APIResponse{Success: true, Data: post}, w, http.StatusOK)
}

func (pc *PostController) GetBySlug(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]
	post, err := pc.PostRepository.FindBySlug(slug)
	if err != nil {
		NewAPIError(&APIError{false, "Could not find post", http.StatusNotFound}, w)
		return
	}

	NewAPIResponse(&APIResponse{Success: true, Data: post}, w, http.StatusOK)
}

func (pc *PostController) Create(w http.ResponseWriter, r *http.Request) {
	uid, err := services.UserIdFromContext(r.Context())
	if err != nil {
		NewAPIError(&APIError{false, "Something went wrong", http.StatusInternalServerError}, w)
		return
	}

	j, err := GetJSON(r.Body)
	if err != nil {
		NewAPIError(&APIError{false, "Invalid request", http.StatusBadRequest}, w)
		return
	}

	title, err := j.GetString("title")
	if err != nil {
		NewAPIError(&APIError{false, "Title is required", http.StatusBadRequest}, w)
		return
	}

	body, err := j.GetString("body")
	if err != nil {
		NewAPIError(&APIError{false, "Content is required", http.StatusBadRequest}, w)
		return
	}

	post := &models.Post{
		Title:     title,
		Slug:      generateSlug(title),
		Body:      body,
		CreatedAt: time.Now(),
		UserID:    uid,
	}

	err = pc.PostRepository.Create(post)
	if err != nil {
		NewAPIError(&APIError{false, "Could not create post", http.StatusBadRequest}, w)
		return
	}

	defer r.Body.Close()
	NewAPIResponse(&APIResponse{Success: true, Message: "Post created"}, w, http.StatusOK)
}

func (pc *PostController) Update(w http.ResponseWriter, r *http.Request) {
	uid, err := services.UserIdFromContext(r.Context())
	if err != nil {
		NewAPIError(&APIError{false, "Something went wrong", http.StatusInternalServerError}, w)
		return
	}
	vars := mux.Vars(r)
	postId, err := strconv.Atoi(vars["id"])
	if err != nil {
		NewAPIError(&APIError{false, "Invalid request", http.StatusBadRequest}, w)
		return
	}
	post, err := pc.PostRepository.FindById(postId)
	if err != nil {
		// post was not found
		NewAPIError(&APIError{false, "Could not find post", http.StatusNotFound}, w)
		return
	}

	j, err := GetJSON(r.Body)
	if err != nil {
		NewAPIError(&APIError{false, "Invalid request", http.StatusBadRequest}, w)
		return
	}

	title, err := j.GetString("title")
	if err != nil {
		NewAPIError(&APIError{false, "Title is required", http.StatusBadRequest}, w)
		return
	}

	body, err := j.GetString("body")
	if err != nil {
		NewAPIError(&APIError{false, "Content is required", http.StatusBadRequest}, w)
		return
	}
	if len(strings.Fields(title)) < 2 {
		NewAPIError(&APIError{false, "Title is too short", http.StatusBadRequest}, w)
		return
	}

	if len(strings.Fields(body)) < 5 {
		NewAPIError(&APIError{false, "Body is too short", http.StatusBadRequest}, w)
		return
	}
	post.UserID = uid
	post.UpdatedAt = mysql.NullTime{Time: time.Now(), Valid: true}
	post.Title = title
	post.Body = body
	post.Slug = generateSlug(title)
	err = pc.PostRepository.Update(post)
	if err != nil {
		NewAPIError(&APIError{false, "Could not update post", http.StatusBadRequest}, w)
		return
	}

	NewAPIResponse(&APIResponse{Success: true, Message: "Post updated"}, w, http.StatusOK)
}
