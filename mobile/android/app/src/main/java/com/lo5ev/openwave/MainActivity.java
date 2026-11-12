package com.lo5ev.openwave;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Build;
import android.webkit.JavascriptInterface;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebChromeClient;
import android.webkit.WebViewClient;

/**
 * Loads the public HTTPS OpenWave service.
 * This avoids architecture/exec issues on newer Samsung devices and keeps
 * the catalog and interface up to date without embedding a local server.
 */
public final class MainActivity extends Activity {
    private static final String OPENWAVE_URL = "https://openwave-ios.onrender.com/";
    private WebView webView;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Color.rgb(5, 5, 5));
        getWindow().setNavigationBarColor(Color.rgb(5, 5, 5));
        if (Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission("android.permission.POST_NOTIFICATIONS") !=
                        android.content.pm.PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 801);
        }

        webView = new WebView(this);
        webView.setBackgroundColor(Color.rgb(5, 5, 5));
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setMediaPlaybackRequiresUserGesture(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        webView.setWebChromeClient(new WebChromeClient());
        webView.addJavascriptInterface(new PlaybackBridge(), "OpenWaveNative");
        webView.setWebViewClient(new WebViewClient() {
            @Override public void onReceivedError(
                    WebView view, int errorCode, String description, String failingUrl) {
                String html = "<meta name='viewport' content='width=device-width'>" +
                        "<body style='background:#050505;color:#f4f5ef;font:16px sans-serif;padding:28px'>" +
                        "<h2>OpenWave не загрузился</h2>" +
                        "<p>Проверь интернет или VPN и попробуй ещё раз.</p>" +
                        "<p style='color:#ff9b9b'>" + description + "</p></body>";
                view.loadDataWithBaseURL(OPENWAVE_URL, html, "text/html", "UTF-8", null);
            }
        });
        setContentView(webView);
        webView.loadUrl(OPENWAVE_URL);
    }

    private final class PlaybackBridge {
        @JavascriptInterface
        public void startAudio() {
            Intent intent = new Intent(MainActivity.this, PlaybackService.class);
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(intent);
            } else {
                startService(intent);
            }
        }

        @JavascriptInterface
        public void stopAudio() {
            stopService(new Intent(MainActivity.this, PlaybackService.class));
        }
    }

    @Override public void onBackPressed() {
        if (webView != null && webView.canGoBack()) webView.goBack();
        else super.onBackPressed();
    }

    @Override protected void onDestroy() {
        if (webView != null) webView.destroy();
        super.onDestroy();
    }
}
