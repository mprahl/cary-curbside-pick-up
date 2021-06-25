# Cary Curbside Pick Up

Cary Curbside Pick Up is an Alexa skill to be executed in AWS lambda to
provide scheduling help for [Cary, North Carolina](https://www.townofcary.org/).

This can be easily modified to any town that uses the
[ReCollect](https://recollect.net/) API.

## Alexa Intents

### GetSchedule

This intent provides the date for the upcoming requested waste pick up type
(e.g. recycling). This intent requires the `collectionType` intent slot which
would contain a waste pick up type such as garbage, recycling, leaf collection,
or yard waste.

A configured utterance might be `when is the next {collectionType} pick up`.

### WhatIsNext

This intent provides the date and the services on the next curbside pick up day.

A configured utterance might be `what is next`.

## Configuration

The [Cary, North Carolina](https://www.townofcary.org/) address must be
configured using the `STREET_ADDRESS` environment variable. An example value
is `1260 NW Maynard Rd`.

## Build

To build the binary and zip it for AWS Lambda, run the following commands:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o main ./main.go
zip handler.zip ./main
```

Then you can upload `handler.zip` to AWS Lambda.
