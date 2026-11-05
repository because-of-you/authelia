// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { act, fireEvent, render, screen } from "@testing-library/react";

import {
    consumeWeChatChallenge,
    getWeChatChallengeStatus,
    isSafeWeChatQRCode,
    startWeChatChallenge,
} from "@services/WeChat";
import WeChatForm from "@views/LoginPortal/FirstFactor/WeChatForm";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@hooks/QueryParam", () => ({
    useQueryParam: (name: string) => (name === "rd" ? "https://app.example.com" : "GET"),
}));
vi.mock("@hooks/Flow", () => ({ useFlow: () => ({ flow: "openid_connect", id: "flow-id", subflow: "" }) }));
vi.mock("@hooks/OpenIDConnect", () => ({ useUserCode: () => undefined }));
vi.mock("@services/WeChat", () => ({
    consumeWeChatChallenge: vi.fn(),
    getWeChatChallengeStatus: vi.fn(),
    isSafeWeChatQRCode: vi.fn(),
    startWeChatChallenge: vi.fn(),
}));

const startMock = vi.mocked(startWeChatChallenge);
const statusMock = vi.mocked(getWeChatChallengeStatus);
const consumeMock = vi.mocked(consumeWeChatChallenge);
const safeQRMock = vi.mocked(isSafeWeChatQRCode);

function renderForm() {
    const props = {
        disabled: false,
        onAuthenticationStart: vi.fn(),
        onAuthenticationStop: vi.fn(),
        onAuthenticationSuccess: vi.fn(),
    };
    return { ...render(<WeChatForm {...props} />), props };
}

beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    safeQRMock.mockReturnValue(true);
    startMock.mockResolvedValue({
        expires_at: "2030-01-01T00:00:00Z",
        qr_code: "/api/qr-login/image?transaction=test-transaction",
        transaction: "test-transaction",
    });
});

afterEach(() => {
    vi.useRealTimers();
});

describe("WeChat sign in", () => {
    it("starts a browser-bound challenge and completes the original flow after confirmation", async () => {
        const { props } = renderForm();
        await act(async () => {
            fireEvent.click(screen.getByText("Sign in with WeChat"));
        });
        expect(props.onAuthenticationStart).toHaveBeenCalledOnce();
        expect(startMock).toHaveBeenCalledWith(
            {
                flow: "openid_connect",
                flowID: "flow-id",
                requestMethod: "GET",
                subflow: "",
                targetURL: "https://app.example.com",
                userCode: undefined,
            },
            expect.any(AbortSignal),
        );
        expect(screen.getByAltText("WeChat sign-in QR code")).toHaveAttribute(
            "src",
            "/api/qr-login/image?transaction=test-transaction",
        );
        statusMock.mockResolvedValueOnce({ status: "pending" }).mockResolvedValueOnce({ status: "approved" });
        consumeMock.mockResolvedValue({ redirect: "https://app.example.com", status: "consumed" });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1800);
        });
        expect(props.onAuthenticationSuccess).not.toHaveBeenCalled();
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1800);
        });
        expect(props.onAuthenticationStop).toHaveBeenCalledOnce();
        expect(consumeMock).toHaveBeenCalledWith("test-transaction", expect.any(AbortSignal));
        expect(props.onAuthenticationSuccess).toHaveBeenCalledWith("https://app.example.com");
    });

    it("stops polling when the user cancels", async () => {
        const { props } = renderForm();
        await act(async () => {
            fireEvent.click(screen.getByText("Sign in with WeChat"));
        });
        const signal = startMock.mock.calls[0][1];
        fireEvent.click(screen.getByText("Cancel"));
        await act(async () => {
            await vi.advanceTimersByTimeAsync(5000);
        });
        expect(signal.aborted).toBe(true);
        expect(statusMock).not.toHaveBeenCalled();
        expect(props.onAuthenticationStop).toHaveBeenCalledOnce();
    });

    it("does not display an untrusted QR image URL", async () => {
        safeQRMock.mockReturnValue(false);
        const { props } = renderForm();
        await act(async () => {
            fireEvent.click(screen.getByText("Sign in with WeChat"));
        });
        expect(screen.getByText("Unable to complete WeChat sign in")).toBeInTheDocument();
        expect(screen.queryByAltText("WeChat sign-in QR code")).not.toBeInTheDocument();
        expect(props.onAuthenticationStop).toHaveBeenCalledOnce();
    });

    it("does not report success if the browser fails to consume an approved challenge", async () => {
        statusMock.mockResolvedValue({ status: "approved" });
        consumeMock.mockRejectedValue(new Error("consume failed"));
        const { props } = renderForm();
        await act(async () => {
            fireEvent.click(screen.getByText("Sign in with WeChat"));
        });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1800);
        });
        expect(props.onAuthenticationSuccess).not.toHaveBeenCalled();
        expect(props.onAuthenticationStop).toHaveBeenCalledOnce();
        expect(screen.getByText("Unable to complete WeChat sign in")).toBeInTheDocument();
    });

    it.each(["rejected", "expired"] as const)("stops on a %s challenge", async (status) => {
        statusMock.mockResolvedValue({ status });
        const { props } = renderForm();
        await act(async () => {
            fireEvent.click(screen.getByText("Sign in with WeChat"));
        });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1800);
        });
        expect(props.onAuthenticationSuccess).not.toHaveBeenCalled();
        expect(props.onAuthenticationStop).toHaveBeenCalledOnce();
        expect(screen.getByText("Try again")).toBeInTheDocument();
    });
});
