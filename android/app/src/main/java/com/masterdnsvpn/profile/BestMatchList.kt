package com.masterdnsvpn.profile

/**
 * Well-known id of the auto-managed "Best Match" resolver list.
 * The list's contents live on disk at <filesDir>/profiles/<BEST_MATCH_LIST_ID>/
 * best_match_resolvers.txt (written by the Go engine from live quality stats)
 * and are mirrored into Room for display.
 */
const val BEST_MATCH_LIST_ID = "best-match"
