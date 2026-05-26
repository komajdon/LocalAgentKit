//go:build linux

package main

/*
#cgo pkg-config: webkit2gtk-4.1
#include <webkit2/webkit2.h>
#include <gtk/gtk.h>

static gboolean on_permission_request(WebKitWebView *wv, WebKitPermissionRequest *req, gpointer data) {
    webkit_permission_request_allow(req);
    return TRUE;
}

static void patch_widget(GtkWidget *widget, gpointer data) {
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        WebKitSettings *s = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget));
        webkit_settings_set_enable_media_stream(s, TRUE);
        webkit_settings_set_enable_media_capabilities(s, TRUE);
        // avoid duplicate signal connections
        if (!g_signal_handler_find(widget, G_SIGNAL_MATCH_FUNC, 0, 0, NULL,
                                   G_CALLBACK(on_permission_request), NULL)) {
            g_signal_connect(widget, "permission-request",
                             G_CALLBACK(on_permission_request), NULL);
        }
        return;
    }
    if (GTK_IS_CONTAINER(widget)) {
        gtk_container_foreach(GTK_CONTAINER(widget),
                              (GtkCallback)patch_widget, NULL);
    }
}

static gboolean do_patch(gpointer data) {
    GList *wins = gtk_window_list_toplevels();
    for (GList *w = wins; w; w = w->next)
        patch_widget(GTK_WIDGET(w->data), NULL);
    g_list_free(wins);
    return FALSE; // one-shot
}

// EnableMediaAccess schedules the patch on the GTK main loop.
// Safe to call from any goroutine — g_idle_add is thread-safe.
void EnableMediaAccess(void) {
    g_idle_add(do_patch, NULL);
}
*/
import "C"

// EnableMediaAccess grants microphone (and other media) permission requests
// inside the embedded WebKit2GTK view. Call this from OnDomReady so the
// WebView widget tree already exists.
func EnableMediaAccess() {
    C.EnableMediaAccess()
}
