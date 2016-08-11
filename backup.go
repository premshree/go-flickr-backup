package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/premshree/go-flickr"
	"github.com/sethgrid/pester"
)

const (
	API_KEY             = "YOUR-API-KEY"
	API_SECRET          = "YOU-API-SECRET"
	PHOTO_SIZE_ORIGINAL = "Original"
	BACKUP_DIR          = "/path/to/backup-dir"
	CONFIG_PATH         = "/path/to/config"
)

type PhotoSetsJson struct {
	Photosets PhotoSets
}

type PhotoSets struct {
	Photoset []PhotoSet
}

type PhotoSet struct {
	Id          string
	Title       Meta
	Description Meta
}

type Meta map[string]string

type PhotosJson struct {
	Photoset PhotosPhotoSet
}

type PhotosPhotoSet struct {
	Id    string
	Photo []Photo
}

type Photo struct {
	Id          string
	Title       string
	Description string
}

type PhotoSizesJson struct {
	Sizes PhotoSizes
}

type PhotoSizes struct {
	Size []PhotoSize
}

type PhotoSize struct {
	Label  string
	Source string
}

type UserJson struct {
	User User
}

type User struct {
	Id       string
	Username map[string]string
}

type OAuthConfig struct {
	OAuthToken       string
	OAuthTokenSecret string
}

type PhotosChannelMessage struct {
	Photo      Photo
	Ok         bool
	PhotoSetId string
	Counts     []int
}

var (
	oauthConfig    *OAuthConfig
	pageNum        *int
	photoSetsCount *int
)

func main() {
	pageNum = flag.Int("page", 1, "Page number of photosets")
	photoSetsCount = flag.Int("photosets", 10, "Number of photoset per run")
	flag.Parse()

	req := NewRequest()

	oauthConfig = GetOAuthConfig()

	if oauthConfig != nil {
		req.OAuth.OAuthToken = oauthConfig.OAuthToken
		req.OAuth.OAuthTokenSecret = oauthConfig.OAuthTokenSecret
	} else {
		token, _ := req.RequestToken()

		// we'll use these later, after user authorization, to exchange for an access token
		oauth_token := token["oauth_token"]
		oauth_token_secret := token["oauth_token_secret"]

		authorizeUrl := req.AuthorizeUrl(token, "read")
		fmt.Println("** Authorize this application at Flickr using the following URL:")
		fmt.Println(authorizeUrl)

		fmt.Println("\n** Enter the oauth_verifier code from the callback URL:")
		reader := bufio.NewReader(os.Stdin)
		oauth_verifier, _ := reader.ReadString('\n')
		oauth_verifier = strings.TrimSpace(oauth_verifier)
		access_token, _ := req.AccessToken(oauth_token, oauth_verifier, oauth_token_secret)

		req.OAuth.OAuthToken = access_token["oauth_token"]
		req.OAuth.OAuthTokenSecret = access_token["oauth_token_secret"]

		// save our token to config.json
		oauthConfig = &OAuthConfig{
			OAuthToken:       req.OAuth.OAuthToken,
			OAuthTokenSecret: req.OAuth.OAuthTokenSecret,
		}
		SaveOAuthConfig(oauthConfig)

	}

	// test login
	req.Method = "flickr.test.login"
	resp, err := req.ExecuteAuthenticated()
	if err != nil {
		fmt.Println("** Error executing method: ", err)
		os.Exit(0)
	}
	var userJson UserJson
	err = json.Unmarshal([]byte(resp), &userJson)
	if err != nil {
		fmt.Println("Error unmarshaling json: ", err)
	}
	msg := fmt.Sprintf("\n** Logged in as %s [%s]", userJson.User.Id, userJson.User.Username["_content"])
	fmt.Println(msg)

	// get all photo sets
	msg = fmt.Sprintf("** Backing up %d photosets, page %d\n", *photoSetsCount, *pageNum)
	fmt.Println(msg)
	start := time.Now()
	req.Method = "flickr.photosets.getList"
	req.Args["user_id"] = userJson.User.Id
	req.Args["page"] = strconv.Itoa(*pageNum)
	req.Args["per_page"] = strconv.Itoa(*photoSetsCount)
	resp, err = req.ExecuteAuthenticated()
	if err != nil {
		fmt.Println("** Error executing method: ", err)
		os.Exit(0)
	}
	var photoSetsJson PhotoSetsJson
	err = json.Unmarshal([]byte(resp), &photoSetsJson)
	if err != nil {
		fmt.Println("Error unmarshaling json: ", err)
	}

	if reflect.DeepEqual(photoSetsJson.Photosets.Photoset, []PhotoSet{}) {
		msg = fmt.Sprintf("No photoset found! Are you sure you have %d photosets?", *pageNum**photoSetsCount)
		fmt.Println(msg)
		os.Exit(0)
	}

	photoSetsChan, photosChan := processPhotoSets(photoSetsJson.Photosets.Photoset)
	processedSetsCount := 0
	photoSetsCount := len(photoSetsJson.Photosets.Photoset)
	totalErrors := 0

	for {
		select {
		case set := <-photoSetsChan:
			msg := fmt.Sprintf("-> Processing photoset %s (count: %s)", set[0], set[1])
			fmt.Println(msg)
		case photoMsg := <-photosChan:
			status := "OK"
			if !photoMsg.Ok {
				status = "FAIL"
			}

			msg = fmt.Sprintf("--> Processed photo %s (%d/%d) [%s] ... %s", photoMsg.Photo.Id, photoMsg.Counts[0], photoMsg.Counts[2], photoMsg.Photo.Title, status)
			fmt.Println(msg)

			// are all photos for a set processed?
			if photoMsg.Counts[0] == photoMsg.Counts[2] {
				msg = fmt.Sprintf("\n++ Finished processing all photos (%d) for set %s [error: %d]\n", photoMsg.Counts[0], photoMsg.PhotoSetId, photoMsg.Counts[1])
				fmt.Println(msg)
				processedSetsCount++
				totalErrors += photoMsg.Counts[1]
			}

			// are all photosets processed?
			if processedSetsCount == photoSetsCount {
				elapsed := time.Since(start)
				msg = fmt.Sprintf("ALL DONE (elaspsed time: %s; total errors: %d)", elapsed, totalErrors)
				fmt.Println(msg)
				os.Exit(0)
			}
		}
	}

}

func processPhotoSets(photosets []PhotoSet) (<-chan []string, <-chan *PhotosChannelMessage) {
	photoSetsChan := make(chan []string)
	photosChan := make(chan *PhotosChannelMessage)

	for _, el := range photosets {
		go func(el PhotoSet) {
			req := NewRequest()
			req.OAuth.OAuthToken = oauthConfig.OAuthToken
			req.OAuth.OAuthTokenSecret = oauthConfig.OAuthTokenSecret
			req.Method = "flickr.photosets.getPhotos"
			req.Args["photoset_id"] = el.Id

			resp, err := req.ExecuteAuthenticated()
			if err != nil {
				fmt.Println("** Error executing method: ", err)
			}
			var photosJson PhotosJson
			err = json.Unmarshal([]byte(resp), &photosJson)
			if err != nil {
				fmt.Println("Error unmarshaling json: ", err)
			}
			count := len(photosJson.Photoset.Photo)
			photoSetsChan <- []string{el.Id, strconv.Itoa(count)}
			processPhotos(photosJson.Photoset, photosChan)
		}(el)
	}
	return photoSetsChan, photosChan
}

func processPhotos(photoSet PhotosPhotoSet, photosChan chan *PhotosChannelMessage) {
	photoSetId := photoSet.Id
	photoCount := 0
	errorCount := 0
	photoSetCount := len(photoSet.Photo)
	for _, photo := range photoSet.Photo {
		go func(photo Photo) {
			req := NewRequest()
			req.OAuth.OAuthToken = oauthConfig.OAuthToken
			req.OAuth.OAuthTokenSecret = oauthConfig.OAuthTokenSecret
			req.Method = "flickr.photos.getSizes"
			req.Args["photo_id"] = photo.Id

			resp, err := req.ExecuteAuthenticated()
			if err != nil {
				fmt.Println("** Error executing method: ", err)
			}
			var photoSizesJson PhotoSizesJson
			err = json.Unmarshal([]byte(resp), &photoSizesJson)
			if err != nil {
				fmt.Println("Error unmarshaling json: ", err)
			}
			photoSetPath := BACKUP_DIR + "/" + photoSetId
			filepath := photoSetPath + "/" + photo.Id + ".jpg"
			photoUrl := getOriginalSize(photoSizesJson)
			err = os.Chdir(BACKUP_DIR)
			if err != nil {
				err = os.Mkdir(BACKUP_DIR, 0755)
				if err != nil {
					fmt.Println("Error creating backup dir ", err)
				}
			}
			err = os.Chdir(photoSetPath)
			if err != nil {
				err = os.Mkdir(photoSetPath, 0755)
				if err != nil {
					fmt.Println("Error creating photoset dir ", err)
				}
			}

			err = retry(5, func() error {
				var err error
				err = downloadFile(filepath, photoUrl)
				return err
			})
			ok := true
			if err != nil {
				msg := fmt.Sprintf("*** Error downloading file id %s, %s: %s", photo.Id, photoUrl, err)
				fmt.Println(msg)
				errorCount++
				ok = false
			}
			photoCount++
			update := &PhotosChannelMessage{
				Photo:      photo,
				Ok:         ok,
				PhotoSetId: photoSetId,
				Counts:     []int{photoCount, errorCount, photoSetCount},
			}
			photosChan <- update
		}(photo)
	}
}

func getOriginalSize(photoSizesJson PhotoSizesJson) string {
	sizes := photoSizesJson.Sizes
	var source string
	for _, v := range sizes.Size {
		if v.Label == PHOTO_SIZE_ORIGINAL {
			source = v.Source
			break
		}
	}
	return source
}

func downloadFile(filepath string, url string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	resp, err := getPester().Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func NewRequest() *flickr.Request {
	req := &flickr.Request{
		ApiKey: API_KEY,
		Args: map[string]string{
			"format":         "json",
			"nojsoncallback": "1",
		},
	}
	req.OAuth = &flickr.OAuth{
		ConsumerSecret: API_SECRET,
		Callback:       "https://go-flickr-backup.herokuapp.com/",
	}

	return req
}

func GetOAuthConfig() *OAuthConfig {
	err := os.Chdir(BACKUP_DIR)
	if err != nil {
		err = os.Mkdir(BACKUP_DIR, 0755)
		if err != nil {
			fmt.Println("Error creating backup dir ", err)
			return nil
		}
	}

	file, err := os.Open(CONFIG_PATH)
	if err != nil {
		fmt.Println("error opening config file", err)
		return nil
	}
	decoder := json.NewDecoder(file)
	config := &OAuthConfig{}
	err = decoder.Decode(&config)
	if err != nil {
		fmt.Println("error decoding config file", err)
		return nil
	}

	return config
}

func SaveOAuthConfig(oauthConfig *OAuthConfig) {
	json, err := json.Marshal(oauthConfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	out, err := os.Create(CONFIG_PATH)
	_, err = io.Copy(out, bytes.NewBuffer(json))
	if err != nil {
		fmt.Println("Error saving config ", err)
	}
}

func getPester() *pester.Client {
	client := pester.New()
	client.Concurrency = 1
	client.Backoff = pester.ExponentialBackoff
	client.MaxRetries = 5
	client.Timeout = time.Duration(60 * 8 * time.Second)
	return client
}

// thanks! https://blog.abourget.net/en/2016/01/04/my-favorite-golang-retry-function/
func retry(attempts int, callback func() error) (err error) {
	for i := 0; ; i++ {
		err = callback()
		if err == nil {
			return nil
		}

		if i >= (attempts - 1) {
			break
		}

	}
	return fmt.Errorf("*** after %d attempts, last error: %s", attempts, err)
}
