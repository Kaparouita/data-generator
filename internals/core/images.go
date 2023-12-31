package core

import (
	"context"
	"data-generator/internals/domain"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Kaparouita/models/models"
	"golang.org/x/sync/semaphore"
)

func (srv *GenerateService) GetRecipesFromJson(file string) ([]models.Recipe, error) {
	jsonFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()

	var recipes []models.Recipe
	byteValue, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &recipes)
	if err != nil {
		return nil, err
	}

	for i := range recipes {
		srv.db.SendToDB(&recipes[i])
		if err != nil {
			fmt.Println(err) // TODO: log error
		}
	}
	return recipes, nil
}

func (srv *GenerateService) AddImages(file string) error {
	jsonFile, err := os.Open(file)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()

	var recipes []models.Recipe
	byteValue, _ := io.ReadAll(jsonFile)
	json.Unmarshal(byteValue, &recipes)

	if len(recipes) == 0 {
		return fmt.Errorf("recipes is empty")
	}
	//0-800
	max := 1600
	min := 1200

	recipes, err = srv.GenerateImages(recipes, min, max)
	if err != nil {
		return err
	}

	err = srv.UpdateJson(recipes)
	if err != nil {
		return err
	}
	fmt.Println("Successfully Generated Images")
	return nil
}

func (srv *GenerateService) GenerateImages(recipes []models.Recipe, index, total int) ([]models.Recipe, error) {
	subscriptionKey := "pZiZU2SOr0nnwE9VjyvFPW8qo4y6WbS2VvsQTKucJiSTRzGUob9gabzd"
	endpoint := "https://api.pexels.com/v1/search"
	var wg sync.WaitGroup
	sem := semaphore.NewWeighted(10)
	ctx := context.Background()

	for i := index; i < total; i++ {
		sem.Acquire(ctx, 1)
		wg.Add(1)
		go func(i int, recipe *models.Recipe) {
			client := &http.Client{}
			fmt.Println()

			defer func() {
				sem.Release(1)
				defer wg.Done()
			}()

			if recipe.Image != "" && recipe.Image != "Not found" {
				fmt.Printf("Recipe %d already has tried to retrieve image\n", i)
				return
			}

			recipe.Title = strings.Replace(recipe.Title, "&amp;", "and", -1)
			query := recipe.Title
			encodedQuery := url.QueryEscape(query)
			uriQuery := endpoint + "?query=" + encodedQuery + "&per_page=1" + "&total_results=2" + "&size=medium"
			tries := 0

			fmt.Printf("Start request for recipe %d with tilte %s\n", i, recipe.Title)
			for (recipe.Image == "" || recipe.Image == "Not found") && tries < 10 {
				// Perform the Web request and get the response
				request, err := http.NewRequest("GET", uriQuery, nil)
				if err != nil {
					fmt.Println(err)
					return
				}

				request.Header.Add("Authorization", subscriptionKey)

				response, err := client.Do(request)
				if err != nil {
					fmt.Println(err)
					return
				}
				defer response.Body.Close()
				// Read the body of the response
				body, err := io.ReadAll(response.Body)
				if err != nil {
					fmt.Println(err)
				}

				type RespError struct {
					Message string `json:"message"`
					Status  int    `json:"status"`
				}
				respErr := RespError{}

				err = json.Unmarshal(body, &respErr)
				if err == nil && respErr.Status == 429 {
					fmt.Printf("Recipe %d has been rate limited, sleeping for %d seconds , try number : %d\n", i, 10, tries)
					time.Sleep(time.Duration(10) * time.Second)
					tries++
					continue
				}
				var searchResponse domain.PexelsResponse
				err = json.Unmarshal(body, &searchResponse)
				if err != nil {
					recipe.Image = "Not found"
					fmt.Println(err)
					return
				}
				if recipe.Image == "" || recipe.Image == "Not found" {
					recipe.Image = "Not found"
					for _, photo := range searchResponse.Photos {
						if photo.Src.Original != "" {
							recipe.Image = photo.Src.Original
							break
						}
					}
				}
			}
			fmt.Printf("Finish request for recipe %d found image %s\n", i, recipe.Image)
		}(i, &recipes[i])
	}
	wg.Wait()
	return recipes, nil
}

func (srv *GenerateService) UpdateJson(recipes []models.Recipe) error {
	file, err := json.MarshalIndent(recipes, "", "")
	if err != nil {
		return err
	}
	err = os.WriteFile("recipes.json", file, 0644)
	if err != nil {
		return err
	}
	return nil
}
