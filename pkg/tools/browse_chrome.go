//go:build !nochrome

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

func init() {
	// chromedp available
}

// fetchWithChromeImpl renders a page using headless Chrome via chromedp
func fetchWithChromeImpl(pageURL, selector string) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create chromedp context with headless options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-images", true),        // Skip images for speed
		chromedp.Flag("disable-javascript", false),    // Enable JS!
		chromedp.Flag("block-new-web-contents", true), // Block popups
		chromedp.WindowSize(1280, 800),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var content string
	var err error

	if selector != "" {
		// Get specific element content
		err = chromedp.Run(taskCtx,
			chromedp.Navigate(pageURL),
			chromedp.WaitReady("body"),
			chromedp.Sleep(2*time.Second), // Wait for JS to render
			chromedp.Text(selector, &content, chromedp.NodeVisible),
		)
	} else {
		// Get full page content
		err = chromedp.Run(taskCtx,
			chromedp.Navigate(pageURL),
			chromedp.WaitReady("body"),
			chromedp.Sleep(2*time.Second), // Wait for JS to render
			chromedp.Evaluate(`
				(() => {
					// Remove script, style, nav, header, footer, aside
					const remove = document.querySelectorAll('script, style, nav, header, footer, aside, noscript, iframe');
					remove.forEach(el => el.remove());
					return document.body.innerText;
				})()
			`, &content),
		)
	}

	if err != nil {
		return "", fmt.Errorf("chrome render failed: %w", err)
	}

	return content, nil
}
