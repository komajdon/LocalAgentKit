//go:build linux && (desktop || production || dev)

package main

/*
#cgo pkg-config: webkit2gtk-4.1
#cgo dev CFLAGS: -DLAK_DEV
#include <string.h>
#include <webkit2/webkit2.h>
#include <gtk/gtk.h>

// Deny every permission except notifications. Audio is recorded natively by
// the Go side, so the web view needs no microphone or camera access.
static gboolean on_permission_request(WebKitWebView *wv, WebKitPermissionRequest *req, gpointer data) {
    if (WEBKIT_IS_NOTIFICATION_PERMISSION_REQUEST(req))
        webkit_permission_request_allow(req);
    else
        webkit_permission_request_deny(req);
    return TRUE;
}

// Only the app's own pages may load in the view: any page loaded there could
// call the Go bindings.
static gboolean app_uri(const char *uri) {
    if (uri == NULL) return FALSE;
    if (g_str_has_prefix(uri, "wails://wails/") || strcmp(uri, "wails://wails") == 0) return TRUE;
    if (strcmp(uri, "about:blank") == 0) return TRUE;
#ifdef LAK_DEV
    if (g_str_has_prefix(uri, "http://localhost:") || g_str_has_prefix(uri, "http://127.0.0.1:")) return TRUE;
#endif
    return FALSE;
}

static gboolean on_decide_policy(WebKitWebView *wv, WebKitPolicyDecision *decision,
                                 WebKitPolicyDecisionType type, gpointer data) {
    if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
        webkit_policy_decision_ignore(decision);
        return TRUE;
    }
    if (type == WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION) {
        WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(
            WEBKIT_NAVIGATION_POLICY_DECISION(decision));
        const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(action));
        if (!app_uri(uri)) {
            webkit_policy_decision_ignore(decision);
            return TRUE;
        }
    }
    return FALSE; // default handling
}

static void patch_widget(GtkWidget *widget, gpointer data) {
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        WebKitSettings *s = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget));
        webkit_settings_set_enable_media_stream(s, FALSE);
        // avoid duplicate signal connections
        if (!g_signal_handler_find(widget, G_SIGNAL_MATCH_FUNC, 0, 0, NULL,
                                   G_CALLBACK(on_permission_request), NULL)) {
            g_signal_connect(widget, "permission-request",
                             G_CALLBACK(on_permission_request), NULL);
            g_signal_connect(widget, "decide-policy",
                             G_CALLBACK(on_decide_policy), NULL);
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

// Schedules the patch on the GTK main loop; g_idle_add is thread-safe.
static void harden_webview(void) {
    g_idle_add(do_patch, NULL);
}
*/
import "C"

// HardenWebView restricts permissions and navigation in the embedded WebKit2GTK
// view. Call it from OnDomReady, once the widget tree exists.
func HardenWebView() {
	C.harden_webview()
}
