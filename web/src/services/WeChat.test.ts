// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { isSafeWeChatQRCode } from "@services/WeChat";

vi.mock("@utils/BasePath", () => ({ getBasePath: () => "/auth" }));

describe("WeChat QR image URL", () => {
    it("accepts the same-origin image endpoint under the configured base path", () => {
        expect(isSafeWeChatQRCode("/auth/api/qr-login/image?transaction=abc")).toBe(true);
        expect(isSafeWeChatQRCode(`${window.location.origin}/auth/api/qr-login/image?transaction=abc`)).toBe(true);
    });

    it("rejects external, data, and wrong-path image URLs", () => {
        expect(isSafeWeChatQRCode("https://evil.example/auth/api/qr-login/image")).toBe(false);
        expect(isSafeWeChatQRCode("//evil.example/auth/api/qr-login/image")).toBe(false);
        expect(isSafeWeChatQRCode("data:image/png;base64,AAAA")).toBe(false);
        expect(isSafeWeChatQRCode("/api/qr-login/image?transaction=abc")).toBe(false);
    });
});
