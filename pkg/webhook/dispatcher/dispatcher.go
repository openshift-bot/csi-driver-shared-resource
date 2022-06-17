package dispatcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/openshift/csi-driver-shared-resource/pkg/webhook/csidriver"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	admissionctl "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const validContentType string = "application/json"

var (
	log             = logf.Log.WithName("dispatcher")
	scheme          = runtime.NewScheme()
	admissionCodecs = serializer.NewCodecFactory(scheme)
)

// Dispatcher struct
type Dispatcher struct {
	hook csidriver.Webhook
	mu   sync.Mutex
}

// NewDispatcher new dispatcher
func NewDispatcher(hook csidriver.Webhook) *Dispatcher {
	return &Dispatcher{
		hook: hook,
	}
}

// HandleRequest http request
func (d *Dispatcher) HandleRequest(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	log.Info("Handling request", "request", r.RequestURI)
	_, err := url.Parse(r.RequestURI)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Error(err, "Couldn't parse request %s", r.RequestURI)
		SendResponse(w, admissionctl.Errored(http.StatusBadRequest, err))
		return
	}

	request, _, err := ParseHTTPRequest(r)
	// Problem parsing an AdmissionReview, so use BadRequest HTTP status code
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Error(err, "Error parsing HTTP Request Body")
		SendResponse(w, admissionctl.Errored(http.StatusBadRequest, err))
		return
	}
	// Valid AdmissionReview, but we can't do anything with it because we do not
	// think the request inside is valid.
	if !d.hook.Validate(request) {
		SendResponse(w,
			admissionctl.Errored(http.StatusBadRequest,
				fmt.Errorf("Not a valid webhook request")))
		return
	}

	SendResponse(w, d.hook.Authorized(request))
	return
}

// SendResponse Send the AdmissionReview.
func SendResponse(w io.Writer, resp admissionctl.Response) {
	encoder := json.NewEncoder(w)
	responseAdmissionReview := admissionv1.AdmissionReview{
		Response: &resp.AdmissionResponse,
	}
	responseAdmissionReview.APIVersion = admissionv1.SchemeGroupVersion.String()
	responseAdmissionReview.Kind = "AdmissionReview"
	err := encoder.Encode(responseAdmissionReview)
	if err != nil {
		log.Error(err, "Failed to encode Response", "response", resp)
		SendResponse(w, admissionctl.Errored(http.StatusInternalServerError, err))
	}
}

func ParseHTTPRequest(r *http.Request) (admissionctl.Request, admissionctl.Response, error) {
	var resp admissionctl.Response
	var req admissionctl.Request
	var err error
	var body []byte
	if r.Body != nil {
		if body, err = ioutil.ReadAll(r.Body); err != nil {
			resp = admissionctl.Errored(http.StatusBadRequest, err)
			return req, resp, err
		}
	} else {
		err := errors.New("request body is nil")
		resp = admissionctl.Errored(http.StatusBadRequest, err)
		return req, resp, err
	}
	if len(body) == 0 {
		err := errors.New("request body is empty")
		resp = admissionctl.Errored(http.StatusBadRequest, err)
		return req, resp, err
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != validContentType {
		err := fmt.Errorf("contentType=%s, expected application/json", contentType)
		resp = admissionctl.Errored(http.StatusBadRequest, err)
		return req, resp, err
	}
	ar := admissionv1.AdmissionReview{}
	if _, _, err := admissionCodecs.UniversalDeserializer().Decode(body, nil, &ar); err != nil {
		resp = admissionctl.Errored(http.StatusBadRequest, err)
		return req, resp, err
	}

	if ar.Request == nil {
		err = fmt.Errorf("no request in request body")
		resp = admissionctl.Errored(http.StatusBadRequest, err)
		return req, resp, err
	}
	resp.UID = ar.Request.UID
	req = admissionctl.Request{
		AdmissionRequest: *ar.Request,
	}
	return req, resp, nil
}
