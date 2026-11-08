package jaklingko

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

// PayFelloTicket completes an automated headless payment for a Fello payment gateway URL using the user's 6-digit PIN.
// It performs:
//  1. GET initial purchase request to extract session and verify state
//  2. POST verify step to open PIN challenge
//  3. POST credentialValidation to verify the 6-digit PIN
//  4. POST ConfirmPin to authorize transaction and receive redirect form
//  5. POST redirect callback to JakLingko core API to finalize ticket settlement
func (c *clientImpl) PayFelloTicket(ctx context.Context, felloURL, pin string) error {
	if felloURL == "" {
		return errors.New("felloURL cannot be empty")
	}
	if len(pin) != 6 {
		return errors.New("fello PIN must be exactly 6 digits")
	}

	parsedURL, err := url.Parse(felloURL)
	if err != nil {
		return fmt.Errorf("invalid fello gateway URL: %w", err)
	}

	ticketID := parsedURL.Query().Get("tID")
	if ticketID == "" {
		return errors.New("fello gateway URL missing tID query parameter")
	}

	felloBase := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	// Resolve user phone number
	phone := ""
	if c.session != nil && c.session.Username != "" {
		phone = c.session.Username
	}
	if strings.HasPrefix(phone, "62") {
		phone = "0" + phone[2:]
	}

	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: tr,
		Jar:       jar,
	}

	const userAgent = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36"

	// 1. Initial GET request to initialize Fello session
	reqGet, err := http.NewRequestWithContext(ctx, http.MethodGet, felloURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	reqGet.Header.Set("User-Agent", userAgent)
	reqGet.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respGet, err := httpClient.Do(reqGet)
	if err != nil {
		return fmt.Errorf("initial fello gateway GET request failed: %w", err)
	}
	_, _ = io.ReadAll(respGet.Body)
	respGet.Body.Close()

	if respGet.StatusCode != http.StatusOK {
		return fmt.Errorf("fello gateway GET returned HTTP %d", respGet.StatusCode)
	}

	// 2. Advance to PIN input step: POST /Api/Wallet/V1/Payment/Purchase/Verify?tID=<tID>
	verifyURL := fmt.Sprintf("%s/Api/Wallet/V1/Payment/Purchase/Verify?tID=%s", felloBase, ticketID)
	reqVerify, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create verify request: %w", err)
	}
	reqVerify.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqVerify.Header.Set("User-Agent", userAgent)
	reqVerify.Header.Set("Referer", felloURL)

	respVerify, err := httpClient.Do(reqVerify)
	if err != nil {
		return fmt.Errorf("fello verify step request failed: %w", err)
	}
	verifyBytes, _ := io.ReadAll(respVerify.Body)
	respVerify.Body.Close()

	if respVerify.StatusCode != http.StatusOK {
		return fmt.Errorf("fello verify step returned HTTP %d", respVerify.StatusCode)
	}

	// Extract fromMember if not known
	if phone == "" {
		reFrom := regexp.MustCompile(`name=["']fromMember["'][^>]*value=["']([^"']+)["']`)
		if matches := reFrom.FindStringSubmatch(string(verifyBytes)); len(matches) > 1 {
			phone = matches[1]
		}
	}

	// 3. Validate PIN: POST /Api/Wallet/V1/Payment/Validation/credentialValidation
	valURL := fmt.Sprintf("%s/Api/Wallet/V1/Payment/Validation/credentialValidation", felloBase)
	valData := url.Values{
		"ticketType": {"purchase"},
		"ticketID":   {ticketID},
		"username":   {phone},
		"credential": {pin},
	}

	reqVal, err := http.NewRequestWithContext(ctx, http.MethodPost, valURL, strings.NewReader(valData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}
	reqVal.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	reqVal.Header.Set("User-Agent", userAgent)
	reqVal.Header.Set("X-Requested-With", "XMLHttpRequest")
	reqVal.Header.Set("Referer", verifyURL)

	respVal, err := httpClient.Do(reqVal)
	if err != nil {
		return fmt.Errorf("fello credential validation request failed: %w", err)
	}
	valBytes, _ := io.ReadAll(respVal.Body)
	respVal.Body.Close()

	var valResult struct {
		ResponseMessage string `json:"responseMessage"`
	}
	if err := json.Unmarshal(valBytes, &valResult); err != nil {
		return fmt.Errorf("failed to parse credential validation response: %w (%s)", err, string(valBytes))
	}

	if strings.ToUpper(valResult.ResponseMessage) != "VALID" {
		return fmt.Errorf("fello PIN validation failed: response '%s'", valResult.ResponseMessage)
	}

	// 4. Confirm PIN: POST /Api/Wallet/V1/Payment/Purchase/ConfirmPin?tID=<tID>
	confirmURL := fmt.Sprintf("%s/Api/Wallet/V1/Payment/Purchase/ConfirmPin?tID=%s", felloBase, ticketID)
	confirmData := url.Values{
		"fromMember": {phone},
		"ticketID":   {ticketID},
		"ticketType": {"purchase"},
		"credential": {pin},
	}

	reqConfirm, err := http.NewRequestWithContext(ctx, http.MethodPost, confirmURL, strings.NewReader(confirmData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create confirm PIN request: %w", err)
	}
	reqConfirm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqConfirm.Header.Set("User-Agent", userAgent)
	reqConfirm.Header.Set("Referer", verifyURL)

	respConfirm, err := httpClient.Do(reqConfirm)
	if err != nil {
		return fmt.Errorf("fello confirm PIN request failed: %w", err)
	}
	confirmBytes, _ := io.ReadAll(respConfirm.Body)
	respConfirm.Body.Close()

	if respConfirm.StatusCode != http.StatusOK {
		return fmt.Errorf("fello confirm PIN returned HTTP %d", respConfirm.StatusCode)
	}

	// 5. Parse redirect form action and input parameters
	confirmHTML := string(confirmBytes)
	reAction := regexp.MustCompile(`(?i)<form[^>]*action=["']([^"']+)["']`)
	actionMatch := reAction.FindStringSubmatch(confirmHTML)
	if len(actionMatch) < 2 {
		return fmt.Errorf("failed to locate redirect form action in Fello response: %s", confirmHTML)
	}
	callbackActionURL := actionMatch[1]

	returnFormData := url.Values{}
	reInputTag := regexp.MustCompile(`(?i)<input\s+[^>]*>`)
	reName := regexp.MustCompile(`(?i)name=["']([^"']+)["']`)
	reValue := regexp.MustCompile(`(?i)value=["']([^"']*)["']`)

	for _, inputTag := range reInputTag.FindAllString(confirmHTML, -1) {
		nameMatches := reName.FindStringSubmatch(inputTag)
		valueMatches := reValue.FindStringSubmatch(inputTag)
		if len(nameMatches) > 1 && len(valueMatches) > 1 {
			returnFormData.Set(nameMatches[1], valueMatches[1])
		}
	}

	// 6. Submit return callback redirect to JakLingko
	reqReturn, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackActionURL, strings.NewReader(returnFormData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create JakLingko return request: %w", err)
	}
	reqReturn.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReturn.Header.Set("User-Agent", userAgent)

	respReturn, err := http.DefaultClient.Do(reqReturn)
	if err != nil {
		return fmt.Errorf("JakLingko return callback failed: %w", err)
	}
	defer respReturn.Body.Close()
	returnBytes, _ := io.ReadAll(respReturn.Body)

	if respReturn.StatusCode != http.StatusOK && respReturn.StatusCode != http.StatusCreated {
		return fmt.Errorf("JakLingko callback returned HTTP %d: %s", respReturn.StatusCode, string(returnBytes))
	}

	if !strings.Contains(string(returnBytes), "PAYMENT_SUCCESS") && !strings.Contains(string(returnBytes), "Pembayaran Berhasil") {
		return fmt.Errorf("payment callback did not report success: %s", string(returnBytes))
	}

	return nil
}
