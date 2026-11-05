// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { Get, Post } from "@services/Client";
import { getBasePath } from "@utils/BasePath";

export interface WeChatChallengeRequest {
    targetURL?: string;
    requestMethod?: string;
    flowID?: string;
    flow?: string;
    subflow?: string;
    userCode?: string;
}

export interface WeChatChallenge {
    transaction: string;
    qr_code: string;
    expires_at: string;
}

export interface WeChatChallengeStatus {
    status: "approved" | "consumed" | "expired" | "pending" | "rejected";
    redirect?: string;
}

function path(): string {
    return `${getBasePath()}/api/qr-login`;
}

export function startWeChatChallenge(request: WeChatChallengeRequest, signal: AbortSignal): Promise<WeChatChallenge> {
    return Post<WeChatChallenge>(`${path()}/start`, request, signal);
}

export function getWeChatChallengeStatus(transaction: string, signal: AbortSignal): Promise<WeChatChallengeStatus> {
    return Get<WeChatChallengeStatus>(`${path()}/status?transaction=${encodeURIComponent(transaction)}`, signal);
}

export function consumeWeChatChallenge(transaction: string, signal: AbortSignal): Promise<WeChatChallengeStatus> {
    return Post<WeChatChallengeStatus>(`${path()}/consume`, { transaction }, signal);
}

// Only a same-origin QR endpoint is permitted by the portal's content security policy.
export function isSafeWeChatQRCode(url: string): boolean {
    try {
        const parsed = new URL(url, window.location.origin);
        return parsed.origin === window.location.origin && parsed.pathname === `${path()}/image`;
    } catch {
        return false;
    }
}
