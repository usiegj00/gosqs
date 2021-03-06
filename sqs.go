//
// goamz - Go packages to interact with the Amazon Web Services.
//
// https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Iron.io
//
// Written by Evan Shaw <evan@iron.io>
//
package sqs

import (
	"encoding/xml"
	"fmt"
	"errors"
	"bytes"
	"io/ioutil"
	// "launchpad.net/goamz/aws"
	"github.com/usiegj00/goamz-aws"
	"net/http"
  _ "net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const DEBUG=false

// The SQS type encapsulates operations with a specific SQS region.
type SQS struct {
	aws.Auth
	aws.Region
	private byte // Reserve the right of using private data.
}

// The Queue type encapsulates operations with an SQS queue.
type Queue struct {
	*SQS
	path string
}

// An Attribute specifies which attribute of a message to set or receive.
type Attribute string

const (
	All                                   Attribute = "All"
	ApproximateNumberOfMessages           Attribute = "ApproximateNumberOfMessages"
	ApproximateNumberOfMessagesNotVisible Attribute = "ApproximateNumberOfMessagesNotVisible"
	VisibilityTimeout                     Attribute = "VisibilityTimeout"
	CreatedTimestamp                      Attribute = "CreatedTimestamp"
	LastModifiedTimestamp                 Attribute = "LastModifiedTimestamp"
	Policy                                Attribute = "Policy"
	MaximumMessageSize                    Attribute = "MaximumMessageSize"
	MessageRetentionPeriod                Attribute = "MessageRetentionPeriod"
	QueueArn                              Attribute = "QueueArn"
)

// New creates a new SQS.
func New(auth aws.Auth, region aws.Region) *SQS {
	return &SQS{auth, region, 0}
}

type ResponseMetadata struct {
	RequestId string
}

func (sqs *SQS) Queue(name string) (q*Queue, err * SqsError) {
	qs, err := sqs.ListQueues(name)
	if err != nil {
		return nil, err
	}
	for _, q := range qs {
		if q.Name() == name {
			return q, nil
		}
	}
	// TODO: return error
	return nil, &SqsError{errors.New("Did not find the queue."),0,"","","","",""}
}

type listQueuesResponse struct {
	Queues []string `xml:"ListQueuesResult>QueueUrl"`
	ResponseMetadata
}

// ListQueues returns a list of your queues.
//
// See http://goo.gl/q1ue9 for more details.
func (sqs *SQS) ListQueues(namePrefix string) (q[]*Queue, err * SqsError) {
	params := url.Values{}
	if namePrefix != "" {
		params.Set("QueueNamePrefix", namePrefix)
	}
	var resp listQueuesResponse
	if err = sqs.get("ListQueues", "/", params, &resp); err != nil {
		return nil, err
	}
	queues := make([]*Queue, len(resp.Queues))
	for i, queue := range resp.Queues {
		u, e := url.Parse(queue)
		if e != nil {
			return nil, &SqsError{e,0,"","","","",""}
		}
		queues[i] = &Queue{sqs, u.Path}
	}
	return queues, nil
}

func (sqs *SQS) newRequest(method, action, url_ string, params url.Values) (*http.Request, error) {
	req, err := http.NewRequest(method, url_, nil)
	if err != nil {
		return nil, err
	}

	params["Action"] = []string{action}
	params["Timestamp"] = []string{time.Now().UTC().Format(time.RFC3339)}
	params["Version"] = []string{"2009-02-01"}

	req.Header.Set("Host", req.Host)

	sign(sqs.Auth, method, req.URL.Path, params, req.Header)
	return req, nil
}

// Error encapsulates an error returned by SDB.
type SqsError struct {
	Error      error  // The proper error
	StatusCode int    // HTTP status code (200, 403, ...)
	StatusMsg  string // HTTP status message ("Service Unavailable", "Bad Request", ...)
	Type       string // Whether the error was a receiver or sender error
	Code       string // SQS error code ("InvalidParameterValue", ...)
	Message    string // The human-oriented error message
	RequestId  string // A unique ID for this request
}

func (err *SqsError) String() string {
	return err.Message
}

func buildError(r *http.Response) (sqsError * SqsError) {
	sqsError = &SqsError{}
	sqsError.StatusCode = r.StatusCode
	sqsError.StatusMsg = r.Status
  body, err := ioutil.ReadAll(r.Body)
  if(err != nil) {
    sqsError.Error = err
    return
  }

  str := bytes.NewBuffer(body)
  if true { // DEBUG {
    fmt.Printf("buildError::body: %s", str.String())
  }

  if(err != nil) {
    sqsError.Error = err
    return
  }
	xml.Unmarshal(body, &sqsError)
	return
}

func (sqs *SQS) doRequest(req *http.Request, resp interface{}) (err * SqsError) {
	// dump, _ := httputil.DumpRequest(req, true)
	// println("req DUMP:\n", string(dump))

	r, e := http.DefaultClient.Do(req)
	if e != nil {
		return &SqsError{e,0,"","","","",""}
	}

	defer r.Body.Close()
	// str, _ := http.DumpResponse(r, true)
	// fmt.Printf("response text: %s\n", str)
	// fmt.Printf("response struct: %+v\n", resp)
	if r.StatusCode != 200 {
		err = buildError(r)
    fmt.Println("sqs.doRequest: Error", r)
    return err
	}

  //fmt.Println("doRequest v4")

  body, e := ioutil.ReadAll(r.Body)

  if DEBUG {
    str := bytes.NewBuffer(body)
    fmt.Printf("Body: %80v\n", str.String())
  }

	if e != nil {
    // fmt.Printf("Err: %80s\n", err)
		return &SqsError{e,0,"","","","",""}
	}

	e = xml.Unmarshal(body, resp)
  if e != nil {
    return &SqsError{e,0,"","","","",""}
  }
  return nil
}

func (sqs *SQS) post(action, path string, params url.Values, resp interface{}) (err *SqsError) {
	endpoint := strings.Replace(sqs.Region.EC2Endpoint, "ec2", "sqs", 1) + path
	req, e := sqs.newRequest("POST", action, endpoint, params)
	if e != nil {
		return &SqsError{e,0,"","","","",""}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	encodedParams := params.Encode()
	req.Body = ioutil.NopCloser(strings.NewReader(encodedParams))
	req.ContentLength = int64(len(encodedParams))

  if DEBUG {
    fmt.Printf("--------------------------------------\n")
    fmt.Printf("%v\n", req)
    fmt.Printf("%s\n", req.Body)
    fmt.Printf("--------------------------------------\n")
  }

	return sqs.doRequest(req, resp)
}

func (sqs *SQS) get(action, path string, params url.Values, resp interface{}) (err * SqsError) {
	if params == nil {
		params = url.Values{}
	}
	endpoint := strings.Replace(sqs.Region.EC2Endpoint, "ec2", "sqs", 1) + path
	req, e := sqs.newRequest("GET", action, endpoint, params)
	if e != nil {
		return &SqsError{e,0,"","","","",""}
	}

	if len(params) > 0 {
		req.URL.RawQuery = params.Encode()
	}

	return sqs.doRequest(req, resp)
}

func (q *Queue) Name() string {
	return path.Base(q.path)
}

// AddPermission adds a permission to a queue for a specific principal.
//
// See http://goo.gl/vG4CP for more details.
func (q *Queue) AddPermission() error {
	return nil
}

// ChangeMessageVisibility changes the visibility timeout of a specified message
// in a queue to a new value.
//
// See http://goo.gl/tORrh for more details.
func (q *Queue) ChangeMessageVisibility() error {
	return nil
}

type CreateQueueOpt struct {
	DefaultVisibilityTimeout int
	MaximumMessageSize       int
}

type createQueuesResponse struct {
	QueueUrl string `xml:"CreateQueueResult>QueueUrl"`
	ResponseMetadata
}

// CreateQueue creates a new queue.
//
// See http://goo.gl/EwNUK for more details.
func (sqs *SQS) CreateQueue(name string, opt *CreateQueueOpt) (q*Queue, err * SqsError) {
	params := url.Values{
		"QueueName": []string{name},
	}
	if opt != nil {
		dvt := strconv.Itoa(opt.DefaultVisibilityTimeout)
		params["DefaultVisibilityTimeout"] = []string{dvt}
		mms := strconv.Itoa(opt.MaximumMessageSize)
		params["MaximumMessageSize"]       = []string{mms}
	}
	var resp createQueuesResponse
	if err := sqs.get("CreateQueue", "/", params, &resp); err != nil {
		return nil, err
	}
	u, e := url.Parse(resp.QueueUrl)
	if e != nil {
		return nil, &SqsError{e,0,"","","","",""}
	}
	return &Queue{sqs, u.Path}, nil
}

// DeleteQueue deletes a queue.
//
// See http://goo.gl/zc45Q for more details.
func (q *Queue) DeleteQueue() * SqsError {
	params := url.Values{}
	var resp ResponseMetadata
	if err := q.SQS.get("DeleteQueue", q.path, params, &resp); err != nil {
		return err
	}
	return nil
}

// DeleteMessage deletes a message from the queue.
//
// See http://goo.gl/t8jnk for more details.
func (q *Queue) DeleteMessage() error {
	return nil
}

/*
<GetQueueAttributesResponse xmlns="http://queue.amazonaws.com/doc/2009-02-01/"><GetQueueAttributesResult><Attribute><Name>ApproximateNumberOfMessages</Name><Value>1</Value></Attribute></GetQueueAttributesResult><ResponseMetadata><RequestId>766ee54d-c531-4980-90cc-938ff3466e1b</RequestId></ResponseMetadata></GetQueueAttributesResponse>

Body: <?xml version="1.0"?>
<ReceiveMessageResponse xmlns="http://queue.amazonaws.com/doc/2009-02-01/"><ReceiveMessageResult></ReceiveMessageResult><ResponseMetadata><RequestId>29faed75-f44b-4424-a3dd-1beb1439c13a</RequestId></ResponseMetadata></ReceiveMessageResponse>

*/

type QueueAttributes struct {
  Id    string `xml:"ResponseMetadata>RequestId"`
	Attributes []struct {
		Name  string
		Value string
	} `xml:"GetQueueAttributesResult>Attribute"`
}

// GetQueueAttributes returns one or all attributes of a queue.
//
// See http://goo.gl/X01zD for more details.
func (q *Queue) GetQueueAttributes(attrs ...Attribute) (*QueueAttributes, *SqsError) {
	params := url.Values{}
	for i, attr := range attrs {
    // AttributeName.1=All
		key := fmt.Sprintf("AttributeName.%d", i+1)
		params[key] = []string{string(attr)}
	}
	var resp QueueAttributes
	if err := q.get("GetQueueAttributes", q.path, params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type Message struct {
	Id   string `xml:"ReceiveMessageResult>Message>MessageId"`
	Body string `xml:"ReceiveMessageResult>Message>Body"`
}

// ReceiveMessage retrieves one or more messages from the queue.
//
// See http://goo.gl/8RLI4 for more details.
func (q *Queue) ReceiveMessage() (*Message, *SqsError) {
	var resp Message
	if err := q.get("ReceiveMessage", q.path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RemovePermission removes a permission from a queue for a specific principal.
//
// See http://goo.gl/5QB9W for more details.
func (q *Queue) RemovePermission() error {
	return nil
}

type sendMessageResponse struct {
	Id string `xml:"SendMessageResult>MessageId"`
	ResponseMetadata
}

// SendMessage delivers a message to the specified queue.
// It returns the sent message's ID.
//
// See http://goo.gl/ThjJG for more details.
func (q *Queue) SendMessage(body string) (sqsId string, err *SqsError) {
	params := url.Values{
		"MessageBody": []string{body},
	}
	var resp sendMessageResponse
  // Gets failed with messages over ~20k (see the doc/example3.xml same as doc/example.xml but bigger. Fails as get)
  if false {
    if err = q.get("SendMessage", q.path, params, &resp); err != nil {
      return
    }
  } else {
    // str := bytes.NewBuffer(body)
    if err = q.post("SendMessage", q.path, params, &resp); err != nil {
      fmt.Printf("Error from SendMessage.\n")
      return
    }
  }
  // fmt.Printf("%s\n", resp)
	return resp.Id, nil
}

// SetQueueAttributes sets one attribute of a queue.
//
// See http://goo.gl/YtIjs for more details.
func (q *Queue) SetQueueAttributes() error {
	return nil
}
