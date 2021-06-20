// Package main is an AWS Lambda function to get the curbside pick up
// services for your Cary home. The input must be an Alexa request. To use this,
// set the "STREET_ADDRESS" to your home's street address
// (e.g. 1260 NW Maynard Rd).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/arienmalec/alexa-go"
	"github.com/aws/aws-lambda-go/lambda"
)

// A serviceOccurrence represents a curbside pick up service on a specific day
type serviceOccurrence struct {
	day  string // Format is in 2021-06-22
	name string // Typical values are Garbage, Recycling, yardwaste, and looseleaf
}

// GetName returns the friendly name of the service occurrence
func (s serviceOccurrence) GetName() string {
	if s.name == "yardwaste" {
		return "Yard Waste"
	} else if s.name == "looseleaf" {
		return "Leaf Collection"
	}
	return s.name
}

// GetFormatted Day returns the friendly day of the occurrence in the format of
// Monday, January 2, 2006
func (s serviceOccurrence) GetFormattedDay() string {
	t, _ := time.Parse("2006-01-02", s.day)
	return t.Format("Monday, January 2, 2006")
}

// handleGetSchedule handles the GetSchedule intent and returns an Alexa
// response
func handleGetSchedule(address string, serviceType string) (alexa.Response, error) {
	occurrences, err := getThirtyDaySchedule(address)
	if err != nil {
		return alexa.Response{}, err
	}

	serviceTypeLower := strings.ToLower(serviceType)
	for _, occurrence := range occurrences {
		if strings.ToLower(occurrence.GetName()) == serviceTypeLower {
			title := fmt.Sprintf("%v Curbside Pick Up", occurrence.GetName())
			msg := fmt.Sprintf("Curbside pick up for %s is on %s.", serviceTypeLower, occurrence.GetFormattedDay())
			return alexa.NewSimpleResponse(title, msg), nil
		}
	}

	title := fmt.Sprintf("%v Curbside Pick Up", serviceType)
	msg := fmt.Sprintf("Curbside pick up for %s is not scheduled in the next 30 days.", serviceType)
	return alexa.NewSimpleResponse(title, msg), nil
}

// handleWhatIsNext handles the WhatIsNext intent and returns an Alexa response
func handleWhatIsNext(address string) (alexa.Response, error) {
	occurrences, err := getThirtyDaySchedule(address)
	if err != nil {
		return alexa.Response{}, err
	}

	var pickUpDate string
	var serviceNames []string
	// occurrences is ordered by date in ascending order
	for i, occurrence := range occurrences {
		if i == 0 {
			pickUpDate = occurrence.GetFormattedDay()
		} else if pickUpDate != occurrence.GetFormattedDay() {
			// Break when the second scheduled pick up date is encountered
			break
		}

		serviceNames = append(serviceNames, occurrence.name)
	}

	if len(serviceNames) == 0 {
		log.Print("No curbside pick up is scheduled in the next 30 days")
		msg := "No curbside pick up is scheduled in the next 30 days."
		response := alexa.NewSimpleResponse("No Curbside Pick Up", msg)
		return response, nil
	}

	log.Printf("Found %d services on %s", len(serviceNames), pickUpDate)
	msg := fmt.Sprintf("On %s, there will be curb side pick up for: ", pickUpDate)
	sort.Strings(serviceNames)
	for i, s := range serviceNames {
		if i != 0 && (i+1) == len(serviceNames) {
			msg += fmt.Sprintf(", and %s", strings.ToLower(s))
		} else if i+1 != len(serviceNames) {
			msg += fmt.Sprintf(", %s", strings.ToLower(s))
		} else {
			msg += fmt.Sprintf(" %s.", strings.ToLower(s))
		}
	}

	return alexa.NewSimpleResponse("Curbside Pick Up Schedule", msg), nil
}

// intentDispatcher handles all incoming Alexa requests and returns an Alexa
// response
func intentDispatcher(ctx context.Context, request alexa.Request) (alexa.Response, error) {
	address := os.Getenv("STREET_ADDRESS")
	if address == "" {
		log.Panic("the address is not configured")
	}
	log.Printf("Using the address %s", address)

	log.Printf("Finding the handler for the intent %s", request.Body.Intent.Name)
	switch request.Body.Intent.Name {
	case "GetSchedule":
		var serviceType string = request.Body.Intent.Slots["collectionType"].Value
		log.Printf("The GetSchedule intent has the service type %s", serviceType)
		return handleGetSchedule(address, serviceType)
	case "WhatIsNext":
		return handleWhatIsNext(address)
	case "AMAZON.HelpIntent":
		const helpMsg string = `You can say things like what's next or when's ` +
			`recycling. The four supported collection types are: ` +
			`garbage, recycling, yard waste, and leaf collection.`
		response := alexa.NewSimpleResponse("Help", helpMsg)
		return response, nil
	default:
		log.Printf("The intent %s was unrecognized", request.Body.Intent.Name)
		response := alexa.NewSimpleResponse("Unknown Request", "The intent was unrecognized")
		return response, nil
	}
}

// getAddressID returns the address ID used by the recollect API
func getAddressID(address string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	addressQS := url.QueryEscape(address)
	url := fmt.Sprintf("https://api.recollect.net/api/areas/CaryNC/services/1087/address-suggest?q=%s", addressQS)
	log.Printf("Making an HTTP request at %s", url)
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("The address lookup HTTP request failed with %s", resp.Status)
		return "", fmt.Errorf("failed to find the address: %s", resp.Status)
	}

	type addressItem struct {
		PlaceID string `json:"place_id"`
	}
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", fmt.Errorf("failed to find the address: %v", err)
	}

	addresses := []addressItem{}
	err = json.Unmarshal(body, &addresses)
	if err != nil {
		log.Printf("Failed to unmarshall the address lookup response: %v", err)
		return "", fmt.Errorf("failed to unmarshall the response: %v", err)
	}

	if len(addresses) == 0 {
		log.Printf("The address %s wasn't found", address)
		return "", errors.New("the address wasn't found")
	}

	// Just return the first found address since it is the most accurrate
	log.Printf("Found the address ID of %s", addresses[0].PlaceID)
	return addresses[0].PlaceID, nil
}

// getThirtyDaySchedule will query the recollect API to find the service
// occurrences in the next 30 days. This returns a slice of serviceOccurrence
// instances.
func getThirtyDaySchedule(address string) ([]serviceOccurrence, error) {
	addressID, err := getAddressID(address)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	after := time.Now().Format("2006-01-02")
	before := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	url := fmt.Sprintf("https://api.recollect.net/api/places/%s/services/1087/events?nomerge=1&hide=reminder_only&after=%s&before=%s", addressID, after, before)
	log.Printf("Making an HTTP request at %s", url)
	resp, err := client.Get(url)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("The schedule lookup failed with %s", resp.Status)
		return nil, fmt.Errorf("failed to get the schedule: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("failed to get the schedule")
	}

	type flag struct {
		Name        string
		ServiceName string `json:"service_name"`
	}
	type event struct {
		Day   string
		Flags []flag
	}
	type eventJSON struct {
		Events []event
	}
	var rvJSON eventJSON
	err = json.Unmarshal(body, &rvJSON)
	if err != nil {
		log.Printf("Failed to unmarshall the schedule lookup response: %v", err)
		return nil, fmt.Errorf("failed to unmarshall the response: %v", err)
	}

	var occurrences []serviceOccurrence
	for _, event := range rvJSON.Events {
		for _, flag := range event.Flags {
			if flag.ServiceName == "waste" {
				occurrence := serviceOccurrence{event.Day, flag.Name}
				occurrences = append(occurrences, occurrence)
				break
			}
		}
	}

	return occurrences, nil
}

// main starts AWS Lambda on the intentDispatcher function
func main() {
	lambda.Start(intentDispatcher)
}
