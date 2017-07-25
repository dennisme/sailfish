package redfishserver

import (
	"context"
	"errors"
	eh "github.com/looplab/eventhorizon"
	"net/http"
	"strings"

	"github.com/superchalupa/go-redfish/domain"

	"fmt"
)

var _ = fmt.Printf

type Response struct {
	// status code is for external users
	StatusCode int
	Headers    map[string]string
	Output     interface{}
}

// Service is the business logic for a redfish server
type Service interface {
	GetRedfishResource(ctx context.Context, r *http.Request, privileges []string) (*Response, error)
	RedfishResourceHandler(ctx context.Context, r *http.Request, privileges []string) (*Response, error)
	domain.DDDFunctions
}

// ServiceMiddleware is a chainable behavior modifier for Service.
type ServiceMiddleware func(Service) Service

var (
	// ErrNotFound is returned when a request isnt present (404)
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("Unauthorized") // 401... missing or bad authentication
	ErrForbidden    = errors.New("Forbidden")    // should be 403 (you are authenticated, but dont have permissions to this object)
)

type RequestAdapter struct {
	Handle func(ctx context.Context, r *http.Request, privileges []string) error
}

// ServiceConfig is where we store the current service data
type ServiceConfig struct {
	// map of METHOD:URI/Type/Context to event
	requestAdapterList map[string]RequestAdapter
	domain.DDDFunctions
}

// NewService is how we initialize the business logic
func NewService(d domain.DDDFunctions) Service {
	cfg := ServiceConfig{
		RequestAdapterList: map[string]RequestAdapter{},
		DDDFunctions:       d,
	}

	return &cfg
}

func (rh *ServiceConfig) RegisterEventAdapter(add string, adapter RequestAdapter) {
	rh.RequestAdapterList[add] = adapter
}

func (rh *ServiceConfig) GetRedfishResource(ctx context.Context, r *http.Request, privileges []string) (*Response, error) {
	noHashPath := strings.SplitN(r.URL.Path, "#", 2)[0]

	// we have the tree ID, fetch an updated copy of the actual tree
	tree, err := domain.GetTree(ctx, rh.GetReadRepo(), rh.GetTreeID())
	if err != nil {
		return &Response{StatusCode: http.StatusInternalServerError, Output: map[string]interface{}{"error": err.Error()}}, err
	}

	// now that we have the tree, look up the actual URI in that tree to find
	// the object UUID, then pull that from the repo
	requested, err := rh.GetReadRepo().Find(ctx, tree.Tree[noHashPath])
	if err != nil {
		return &Response{StatusCode: http.StatusNotFound, Output: map[string]interface{}{"error": err.Error()}}, nil
	}
	item, ok := requested.(*domain.RedfishResource)
	if !ok {
		return &Response{StatusCode: http.StatusInternalServerError}, errors.New("Expected a RedfishResource, but got something strange.")
	}

	return &Response{StatusCode: http.StatusOK, Output: item.Properties, Headers: item.Headers}, nil
}

func (rh *ServiceConfig) RedfishResourceHandler(ctx context.Context, r *http.Request, privileges []string) (*Response, error) {
	// we shouldn't actually ever get a path with a hash, I don't think.
	noHashPath := strings.SplitN(r.URL.Path, "#", 2)[0]
	search := []string{method + ":uri:" + noHashPath}

	// we have the tree ID, fetch an updated copy of the actual tree
	// this should never fail, if it does, something is wrong
	tree, err := domain.GetTree(ctx, rh.GetReadRepo(), rh.GetTreeID())
	if err != nil {
		return &Response{StatusCode: http.StatusInternalServerError}, err
	}

	// now that we have the tree, look up the actual URI in that tree to find
	// the object UUID, then pull that from the repo this can fail if UUID
	// doesn't exist, but request could still be possible if we have a handler
	// for that URI path (without an object)
	requested, err := rh.GetReadRepo().Find(ctx, tree.Tree[noHashPath])
	item, ok := requested.(*domain.RedfishResource)
	if ok {
		aggregateID, ok := item.Properties["@odata.id"].(string)
		if ok {
			search = append(search, method+":id:"+aggregateID)
		}

		typ, ok := item.Properties["@odata.type"].(string)
		if ok {
			search = append(search, method+":type:"+typ)
		}

		context, ok := item.Properties["@odata.context"].(string)
		if ok {
			search = append(search, method+":context:"+contex)
		}
	}

	cmdUUID := eh.NewUUID()

	// we send a command and then wait for a completion event. Set up the wait here.
	// HAVE TO DO THIS FIRST before spitting out command, or we race and may miss the result
	waitID, resultChan := rh.GetEventWaiter().SetupWait(func(event eh.Event) bool {
		if event.EventType() != domain.HTTPCmdProcessedEvent {
			return false
		}
		if data, ok := event.Data().(*domain.HTTPCmdProcessedData); ok {
			if data.CommandID == cmdUUID {
				return true
			}
		}
		return false
	})

	defer rh.GetEventWaiter().CancelWait(waitID)

	for _, s := range search {
		// Ok, so now we look up to see if we have an adapter that can turn this
		// request into an event that we publish on the event bus
		if f, ok := rh.RequestAdapterList[s]; ok {
			// Handle should emit an event, and do processing in a Saga
			// however, we can do more substantial processing here, if we absolutely need to.
			// for example, streaming would need to be handled here
			err := f.Handle(ctx, r, privileges, tree, requested)
			// if handle returns an error, let somebody else have a chance
			if err != nil {
				continue
			}

			goto waitForResponse
		}
	}

	// we didn't find an acceptable method, so pack it all in
	return &Response{StatusCode: http.StatusMethodNotAllowed, Output: map[string]interface{}{"error": err.Error()}}, nil

waitForResponse:
	select {
	case event := <-resultChan:
		d := event.Data().(*domain.HTTPCmdProcessedData)
		return &Response{Output: d.Results, StatusCode: d.StatusCode, Headers: d.Headers}, nil
		// This is an example of how we would set up a job if things time out
		//	case <-time.After(1 * time.Second):
		//		// TODO: Here we could easily automatically create a JOB and return that.
		//		return &Response{StatusCode: http.StatusOK, Output: "JOB"}, nil
	case <-ctx.Done():
		// the requestor cancelled the http request to us. We can abandon
		// returning results, but command will still be processed
		return &Response{StatusCode: http.StatusBadRequest}, nil
	}
}
