package googleworkspace

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/mitchellh/go-homedir"

	"google.golang.org/api/googleapi"
)

// If the argument is a path, pathOrContents loads it and returns the contents,
// otherwise the argument is assumed to be the desired contents and is simply
// returned.
//
// The boolean second return value can be called `wasPath` - it indicates if a
// path was detected and a file loaded.
func pathOrContents(poc string) (string, bool, error) {
	if len(poc) == 0 {
		return poc, false, nil
	}

	path := poc
	if path[0] == '~' {
		var err error
		path, err = homedir.Expand(path)
		if err != nil {
			return path, true, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return string(contents), true, err
		}
		return string(contents), true, nil
	}

	return poc, false, nil
}

// Check Error Code
func isApiErrorWithCode(err error, errCode int) bool {
	gerr, ok := errwrap.GetType(err, &googleapi.Error{}).(*googleapi.Error)
	return ok && gerr != nil && gerr.Code == errCode
}

func handleNotFoundError(err error, d *schema.ResourceData, resource string) diag.Diagnostics {
	if isApiErrorWithCode(err, 404) {
		log.Printf("[WARN] Removing %s because it's gone", resource)
		// The resource doesn't exist anymore
		d.SetId("")

		return nil
	}

	return diag.Errorf("Error when reading or editing %s: %s", resource, err.Error())
}

// This is a Printf sibling (Nprintf; Named Printf), which handles strings like
// Nprintf("Hello %{target}!", map[string]interface{}{"target":"world"}) == "Hello world!".
// This is particularly useful for generated tests, where we don't want to use Printf,
// since that would require us to generate a very particular ordering of arguments.
func Nprintf(format string, params map[string]interface{}) string {
	for key, val := range params {
		format = strings.Replace(format, "%{"+key+"}", fmt.Sprintf("%v", val), -1)
	}
	return format
}

// This will translate a snake cased string to a camel case string
// Note: the first letter of the camel case string will be lower case
func SnakeToCamel(s string) string {
	titled := strings.Title(strings.ReplaceAll(s, "_", " "))
	cameled := strings.Join(strings.Split(titled, " "), "")

	// Lower the first letter
	result := []rune(cameled)
	result[0] = unicode.ToLower(result[0])
	return string(result)
}

// This will translate a snake cased string to a camel case string
// Note: the first letter of the camel case string will be lower case
func CameltoSnake(s string) string {
	var res = make([]rune, 0, len(s))
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			res = append(res, '_', unicode.ToLower(r))
		} else {
			res = append(res, unicode.ToLower(r))
		}
	}
	return string(res)
}

// For resources that have many nested interfaces, we can pass them to the API as is,
// only each field name needs to be camel case rather than snake case. Additionally,
// fields that are not set should not be sent to the API.
func expandInterfaceObjects(parent interface{}) []interface{} {
	objList := parent.([]interface{})
	if len(objList) == 0 {
		return nil
	}

	newObjList := []interface{}{}

	for _, o := range objList {
		obj := o.(map[string]interface{})
		for k, v := range obj {
			if strings.Contains(k, "_") || v == "" {
				delete(obj, k)

				// In the case that the field is not set, don't send it to the API
				if v == "" {
					continue
				}

				obj[SnakeToCamel(k)] = v
			}
		}
		newObjList = append(newObjList, obj)
	}

	return newObjList
}

// For resources that have many nested interfaces, we can set was was returned from the API as is
// only the field names need to be snake case rather than the camel case that is returned
func flattenInterfaceObjects(objList interface{}) interface{} {
	if objList == nil || len(objList.([]interface{})) == 0 {
		return nil
	}

	newObjList := []map[string]interface{}{}

	for _, o := range objList.([]interface{}) {
		obj := o.(map[string]interface{})
		for k, v := range obj {
			delete(obj, k)
			obj[CameltoSnake(k)] = v
		}

		newObjList = append(newObjList, obj)
	}

	return newObjList
}

// Converts a list of interfaces to a list of strings
func listOfInterfacestoStrings(v interface{}) []string {
	result := []string{}

	if v == nil {
		return result
	}

	for _, s := range v.([]interface{}) {
		result = append(result, s.(string))
	}

	return result
}

func stringInSlice(arr []string, str string) bool {
	for _, i := range arr {
		if i == str {
			return true
		}
	}

	return false
}

// sort a slice of interfaces regardless the type, return the equivalent slice of strings
func sortListOfInterfaces(v []interface{}) []string {
	newVal := make([]string, len(v))
	for idx, attr := range v {
		kind := reflect.ValueOf(v).Kind()
		if kind == reflect.Float64 {
			attr = strconv.FormatFloat(attr.(float64), 'f', -1, 64)
		}
		newVal[idx] = fmt.Sprintf("%+v", attr)
	}
	sort.Strings(newVal)
	return newVal
}

func retry(ctx context.Context, retryFunc func() error, duration time.Duration) error {
	return retryTime(ctx, retryFunc, duration, false, false, false)
}

func retryNotFound(ctx context.Context, retryFunc func() error, duration time.Duration) error {
	return retryTime(ctx, retryFunc, duration, true, false, false)
}

func retryTime(ctx context.Context, retryFunc func() error, duration time.Duration, retryNotFound bool, retryPassDuplicate bool, retryInvalid bool) error {
	wait := 1
	return resource.RetryContext(ctx, duration, func() *resource.RetryError {
		err := retryFunc()
		if err == nil {
			return nil
		}

		rand.Seed(time.Now().UnixNano())
		randomNumberMiliseconds := rand.Intn(1001)

		if gerr, ok := err.(*googleapi.Error); ok && (gerr.Code == 500 || gerr.Code == 502 || gerr.Code == 503) {
			log.Printf("[DEBUG] Retrying server error code...")
			time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
			wait = wait * 2
			return resource.RetryableError(gerr)
		}

		hasErrors := false
		gerr, ok := err.(*googleapi.Error)
		if ok && len(gerr.Errors) > 0 {
			hasErrors = true
		}

		if hasErrors && gerr.Errors[0].Reason == "quotaExceeded" {
			log.Printf("[DEBUG] Retrying quota/server error code...")
			time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
			wait = wait * 2
			return resource.RetryableError(gerr)
		}

		if retryPassDuplicate {
			if gerr, ok := err.(*googleapi.Error); ok && (gerr.Code == 401 || gerr.Code == 429) {
				log.Printf("[DEBUG] Retrying quota/server error code...")
				time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
				wait = wait * 2
				return resource.RetryableError(gerr)
			}
		} else {
			if gerr, ok := err.(*googleapi.Error); ok && (gerr.Code == 401 || gerr.Code == 409 || gerr.Code == 429) {
				log.Printf("[DEBUG] Retrying quota/server error code...")
				time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
				wait = wait * 2
				return resource.RetryableError(gerr)
			}
		}

		if retryNotFound {
			if gerr, ok := err.(*googleapi.Error); ok && (gerr.Code == 404) {
				log.Printf("[DEBUG] Retrying for eventual consistency...")
				time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
				wait = wait * 2
				return resource.RetryableError(gerr)
			}
		}

		if retryInvalid {
			if gerr, ok := err.(*googleapi.Error); ok && (gerr.Code == 400) {
				log.Printf("[DEBUG] Retrying invalid error code...")
				time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
				wait = wait * 2
				return resource.RetryableError(gerr)
			}
			if hasErrors && gerr.Errors[0].Reason == "invalid" {
				log.Printf("[DEBUG] Retrying invalid error reason...")
				time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
				wait = wait * 2
				return resource.RetryableError(gerr)
			}
		}

		// Deal with the broken API
		if strings.Contains(fmt.Sprintf("%s", err), "Invalid Input: Bad request for \"") && strings.Contains(fmt.Sprintf("%s", err), "\"code\":400") {
			log.Printf("[DEBUG] Retrying invalid response from API")
			time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
			wait = wait * 2
			return resource.RetryableError(err)
		}
		if strings.Contains(fmt.Sprintf("%s", err), "Service unavailable. Please try again") {
			log.Printf("[DEBUG] Retrying service unavailable from API")
			time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
			wait = wait * 2
			return resource.RetryableError(err)
		}
		if strings.Contains(fmt.Sprintf("%s", err), "Eventual consistency. Please try again") {
			log.Printf("[DEBUG] Retrying due to eventual consistency")
			time.Sleep(time.Duration(wait)*time.Second + time.Duration(randomNumberMiliseconds)*time.Millisecond)
			wait = wait * 2
			return resource.RetryableError(err)
		}

		return resource.NonRetryableError(err)
	})
}
