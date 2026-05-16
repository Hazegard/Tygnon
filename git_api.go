package main

type GitApi interface {
	GetLatestTagId(projectUrl string) (string, error)
	GetLatestReleaseId(projectUrl string) (string, error)
	GetLatestVersion(homepage string, onMaster bool) (string, error)
	GetDescription(homepage string) (string, error)
	HttpGet(assetUrl string, token string) ([]byte, error)
	GetMasterVersionId(projectUrl string) (string, error)
}
