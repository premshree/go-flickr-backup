### Intoduction
Go Flickr Backup is a simple tool to backup your Flickr photos from photosets. It doesn't do everything, but you should be able to modify it fairly easily.

### Requirements
This tool requires [`go-flickr`](https://github.com/premshree/go-flickr), which is forked from the [original](https://github.com/mncaudill/go-flickr) to add OAuth support. This tool also requires [`pester`](https://github.com/sethgrid/pester), which is a nifty little library that allows HTTP retries (and more).

### Installing
Make sure to install the requisites:
```
go get github.com/premshree/go-flickr
go get github.com/sethgrid/pester
```
Simply clone this repository first. Before you build `backup`, make sure to edit the following settings:
```
const (
        API_KEY             = "YOUR-API-KEY"
        API_SECRET          = "YOU-API-SECRET"
        PHOTO_SIZE_ORIGINAL = "Original"
        BACKUP_DIR          = "/path/to/backup-dir"
        CONFIG_PATH         = "/path/to/config"
)
```
Once you've done that, you are ready to build:
```
go build backup.go
```

### Screenshots
##### First run
The first time you run `backup`, it will need you to authorize the tool with Flickr with `read` permissions. This is so the tool can backup private photosets and photos.

![image](https://cloud.githubusercontent.com/assets/149517/17564969/627a989a-5f03-11e6-8a23-1eed86f44d3d.png)
Simply follow the link generated there. You'll be taken to a page on Flickr that looks like so:

![image](https://cloud.githubusercontent.com/assets/149517/17565127/12e9115c-5f04-11e6-9511-232c2d211116.png)

##### Backing up photosets
Once you authenticate with Flickr, flickr-backup stores the OAuth Token and Secret in a config file that it can reuse for future runs.

![image](https://cloud.githubusercontent.com/assets/149517/17564684/436f723c-5f02-11e6-824f-810a4bbc352a.png)

`backup` will spawn multiple `goroutine`s to process your photosets and download photos for those sets. At the end up of the run, the program will exit, with some information on total errors during the run.

![image](https://cloud.githubusercontent.com/assets/149517/17565670/5ad1d790-5f06-11e6-904e-d03847e68acb.png)

Note: by default, `backup` will download `10` photosets at page `1`. (If you have a total of `30` photosets, you have `3` "pages".) You can explicitly specify the page and photoset like so:
```
./backup -photosets=10 -page=26
```

