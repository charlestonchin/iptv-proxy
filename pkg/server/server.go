/*
 * Iptv-Proxy is a project to proxyfie an m3u file and to proxyfie an Xtream iptv service (client API).
 * Copyright (C) 2020  Pierre-Emmanuel Jacquier
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package server

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/jamesnetherton/m3u"
	"github.com/pierre-emmanuelJ/iptv-proxy/pkg/config"
	uuid "github.com/satori/go.uuid"

	"github.com/gin-gonic/gin"
)

var defaultProxyfiedM3UPath = filepath.Join(os.TempDir(), uuid.NewV4().String()+".iptv-proxy.m3u")
var endpointAntiColision = strings.Split(uuid.NewV4().String(), "-")[0]

// Config represent the server configuration
type Config struct {
	*config.ProxyConfig

	// M3U service part
	playlist *m3u.Playlist
	// this variable is set only for m3u proxy endpoints
	track *m3u.Track
	// path to the proxyfied m3u file
	proxyfiedM3UPath string

	endpointAntiColision string
}

// NewServer initialize a new server configuration
func NewServer(config *config.ProxyConfig) (*Config, error) {
	var p m3u.Playlist
	if config.RemoteURL.String() != "" {
		var err error
		p, err = m3u.Parse(config.RemoteURL.String())
		if err != nil {
			return nil, err
		}
	}
	if trimmedCustomId := strings.Trim(config.CustomId, "/"); trimmedCustomId != "" {
		endpointAntiColision = trimmedCustomId
	}
	return &Config{
		config,
		&p,
		nil,
		defaultProxyfiedM3UPath,
		endpointAntiColision,
	}, nil
}

// Serve the iptv-proxy api
func (c *Config) Serve() error {
	if err := c.playlistInitialization(); err != nil {
		return err
	}

	router := gin.Default()
	router.Use(cors.Default())
	group := router.Group("/")
	c.routes(group)

	return router.Run(fmt.Sprintf(":%d", c.HostConfig.Port))
}

func (c *Config) playlistInitialization() error {
	if len(c.playlist.Tracks) == 0 {
		return nil
	}

	f, err := os.Create(c.proxyfiedM3UPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return c.marshallInto(f, false)
}

// MarshallInto a *bufio.Writer a Playlist.
func (c *Config) marshallInto(into *os.File, xtream bool) error {
	filteredTrack := make([]m3u.Track, 0, len(c.playlist.Tracks))

	ret := 0
	excludeURISyntax := strings.Split(c.M3UExcludeURI, ",")
	excludeInfoSyntax := strings.Split(c.M3UExcludeInfo, ",")
	excludeKeyValue := strings.Split(c.M3UExcludeKeyTag, ",")
	excludeMatch := false

	into.WriteString("#EXTM3U\n") // nolint: errcheck
	for i, track := range c.playlist.Tracks {
		var buffer bytes.Buffer
		excludeMatch = false
		buffer.WriteString("#EXTINF:")                       // nolint: errcheck
		buffer.WriteString(fmt.Sprintf("%d ", track.Length)) // nolint: errcheck
		for i := range track.Tags {
			if i == len(track.Tags)-1 {
				buffer.WriteString(fmt.Sprintf("%s=%q", track.Tags[i].Name, track.Tags[i].Value)) // nolint: errcheck
				continue
			}
			buffer.WriteString(fmt.Sprintf("%s=%q ", track.Tags[i].Name, track.Tags[i].Value)) // nolint: errcheck
		}
        // log.Printf("marshallInto() Before URL: %s", track.URI)
		//log.Printf("marshallInto() Before TAG: %s", track.Tags)

		// log.Printf("testcc M3UExcludeNAME: %s", c.M3UExcludeNAME )
		// log.Printf("testcc M3UExcludeURI: %s", c.M3UExcludeURI )
		//log.Printf("testcc M3UIncludeURI: %s", c.M3UIncludeURI )
		// if len(includeURISyntax) > 1 {
		// 	for _, addurl := range includeURISyntax {
		// 		if strings.Contains(track.URI, addurl) { 
		// 			log.Printf("testcc M3UIncludeeURI: %s", addurl )
		// 			notincludeMatch = true
		// 			break 
		// 		}
		// 	}
		// 	if !notincludeMatch {
		// 		ret++
		// 		continue
		// 	}
		// }

		for _, matchingInfo := range excludeInfoSyntax {
			if strings.Contains(track.Name, matchingInfo) { 
				excludeMatch = true
				// log.Printf("excludeInfoSyntax match!")
				break 
			} 
		}
		if excludeMatch {
			ret++
			continue
		}
		for _, matchingURI := range excludeURISyntax {
			if strings.Contains(track.URI, matchingURI ) { 
				excludeMatch = true
				// log.Printf("excludeURISyntax match!")
				break
			} 
		}
		if excludeMatch {
			ret++
			continue
		}
		//log.Printf("marshallInto() Before URL: %s", track.URI)
		uri, err := c.replaceURL(track.URI, i-ret, xtream)
		if err != nil {
			ret++
			log.Printf("ERROR: c.replaceURL() : %s: %s", track.Name, err)
			continue
		}
		//log.Printf("marshallInto() After URL: %s", uri)
		//log.Printf("marshallInto() WriteString URL: %s", fmt.Sprintf("%s, %s\n%s\n", buffer.String(), track.Name, uri) )
		into.WriteString(fmt.Sprintf("%s, %s\n%s\n", buffer.String(), track.Name, uri)) // nolint: errcheck
		for  index, t := range track.Tags {
			for _, matchingKeyValue := range excludeKeyValue {
				if strings.Contains(t.Name, matchingKeyValue){
					// log.Printf("marshallInto() TVG: %s", t.Name)
					track.Tags = append(track.Tags[:index], track.Tags[index+1:]...)
					break
				}
			}
			
			
		}
		filteredTrack = append(filteredTrack, track)
		//log.Printf("marshallInto() After TAG: %s", track.Tags)
	}
	c.playlist.Tracks = filteredTrack
	
	return into.Sync()
}

// ReplaceURL replace original playlist url by proxy url
func (c *Config) replaceURL(uri string, trackIndex int, xtream bool) (string, error) {
	oriURL, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	protocol := "http"
	if c.HTTPS {
		protocol = "https"
	}

	customEnd := strings.Trim(c.CustomEndpoint, "/")
	if customEnd != "" {
		customEnd = fmt.Sprintf("/%s", customEnd)
	}

	uriPath := oriURL.EscapedPath()
	if xtream {
		uriPath = strings.ReplaceAll(uriPath, c.XtreamUser.PathEscape(), c.User.PathEscape())
		uriPath = strings.ReplaceAll(uriPath, c.XtreamPassword.PathEscape(), c.Password.PathEscape())
	} else {
		// if strings.Contains(uri, "/series/"){
		// 	streamType = "series"
		// } else if strings.Contains(uri, "/movie/") {
		// 	streamType = "movie"
		// } else {
		// 	streamType = "live"
		// }
		uriPath = path.Join("/",  c.endpointAntiColision ,c.User.PathEscape(), c.Password.PathEscape(), fmt.Sprintf("%d", trackIndex), path.Base(uriPath))
	}

	basicAuth := oriURL.User.String()
	if basicAuth != "" {
		basicAuth += "@"
	}

	newURI := fmt.Sprintf(
		"%s://%s%s:%d%s%s",
		protocol,
		basicAuth,
		c.HostConfig.Hostname,
		c.AdvertisedPort,
		customEnd,
		uriPath,
	)
    // log.Printf("generate server URL: %s", newURI )
	newURL, err := url.Parse(newURI)
	if err != nil {
		return "", err
	}

	return newURL.String(), nil
}
