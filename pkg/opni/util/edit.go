package cliutil

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/rancher/opni/apis/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var (
	ErrCanceled           = fmt.Errorf("operation canceled")
	ErrNoEditor           = fmt.Errorf("EDITOR environment variable not set")
	ErrObjectDecodeFailed = fmt.Errorf("object decode failed")
)

// EditObject edits an object in a temporary file using the user's default
// editor. It will retry on basic yaml decoding errors but will not validate
// the object against the schema. On success, the provided object will be
// modified in-place. If an error occurs, it will not be modified.
func EditObject(obj runtime.Object, scheme *runtime.Scheme) (runtime.Object, error) {
	// send the yaml to $EDITOR for editing
	editor, ok := os.LookupEnv("EDITOR")
	if !ok {
		return nil, ErrNoEditor
	}

	// edit the object
	defaultHeaders := []string{
		"# Edit the object below. Lines beginning with '#' are ignored.",
		"# If everything is removed, the operation will be canceled.",
	}
	errHeaders := []string{}
	for {
		headers := append([]string{}, defaultHeaders...)
		headers = append(headers, errHeaders...)
		newObj, err := doEdit(editor, obj.DeepCopyObject(), scheme, headers...)
		if err != nil {
			if errors.Is(err, ErrCanceled) {
				return nil, err
			}
			if errors.Is(err, ErrObjectDecodeFailed) {
				errHeaders = []string{"#", fmt.Sprintf("# %s", err.Error())}
				continue
			}
			return nil, err
		}
		return newObj, nil
	}
}

func doEdit(
	editor string,
	obj runtime.Object,
	scheme *runtime.Scheme,
	headerLines ...string,
) (runtime.Object, error) {
	f, err := os.CreateTemp("", "opni-cluster-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	// set the GVK so it shows up in the encoded yaml
	obj.GetObjectKind().SetGroupVersionKind(
		v1beta1.GroupVersion.WithKind("OpniCluster"))
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme,
		json.SerializerOptions{
			Yaml:   true,
			Pretty: true,
		})

	buf := new(bytes.Buffer)
	err = s.Encode(obj, buf)
	if err != nil {
		return nil, err
	}
	// write headers
	for _, line := range headerLines {
		fmt.Fprintln(f, line)
	}
	fmt.Fprint(f, "\n")
	if _, err := f.Write(buf.Bytes()); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	// launch the editor
	cmd := exec.Command(editor, f.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	// read the edited yaml
	time.Sleep(100 * time.Millisecond)
	newData, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	gvk := v1beta1.GroupVersion.WithKind("OpniCluster")
	_, _, err = s.Decode(newData, &gvk, obj)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrObjectDecodeFailed, err.Error())
	}
	return obj, nil
}
