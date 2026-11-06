// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";

import { CircleCheck, QrCode, RefreshCw, Smartphone } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@components/UI/Button";
import { Spinner } from "@components/UI/Spinner";
import { RedirectionURL, RequestMethod } from "@constants/SearchParams";
import { useFlow } from "@hooks/Flow";
import { useUserCode } from "@hooks/OpenIDConnect";
import { useQueryParam } from "@hooks/QueryParam";
import {
    consumeWeChatChallenge,
    getWeChatChallengeStatus,
    isSafeWeChatQRCode,
    startWeChatChallenge,
} from "@services/WeChat";

interface Props {
    disabled: boolean;
    onAuthenticationStart: () => void;
    onAuthenticationStop: () => void;
    onAuthenticationSuccess: (_redirectURL: string | undefined) => void;
}

type Phase = "error" | "expired" | "idle" | "rejected" | "scanned" | "starting" | "success" | "waiting";

const WeChatForm = function (props: Props) {
    const { t: translate } = useTranslation();
    const { onAuthenticationStop } = props;
    const targetURL = useQueryParam(RedirectionURL);
    const requestMethod = useQueryParam(RequestMethod);
    const { flow, id: flowID, subflow } = useFlow();
    const userCode = useUserCode();
    const [phase, setPhase] = useState<Phase>("idle");
    const [qrCode, setQRCode] = useState("");
    const [expiresAt, setExpiresAt] = useState(0);
    const controllerRef = useRef<AbortController | null>(null);
    const timerRef = useRef<null | ReturnType<typeof setTimeout>>(null);
    const autoStartedRef = useRef(false);
    const startRef = useRef<() => Promise<void>>(async () => {});

    const clearAttempt = () => {
        controllerRef.current?.abort();
        controllerRef.current = null;
        if (timerRef.current !== null) {
            clearTimeout(timerRef.current);
            timerRef.current = null;
        }
    };

    useEffect(() => () => clearAttempt(), []);

    const start = async () => {
        if (phase === "starting" || phase === "waiting" || phase === "scanned" || phase === "success") return;
        autoStartedRef.current = true;
        clearAttempt();
        const controller = new AbortController();
        controllerRef.current = controller;
        setQRCode("");
        setPhase("starting");
        props.onAuthenticationStart();

        try {
            const challenge = await startWeChatChallenge(
                { flow, flowID, requestMethod, subflow, targetURL, userCode },
                controller.signal,
            );
            if (controller.signal.aborted) return;
            const expiry = Date.parse(challenge.expires_at);
            if (
                !challenge.transaction ||
                !isSafeWeChatQRCode(challenge.qr_code, challenge.transaction) ||
                !Number.isFinite(expiry)
            ) {
                throw new Error("Invalid WeChat challenge");
            }
            if (expiry <= Date.now()) {
                throw new Error("Expired WeChat challenge");
            }
            setExpiresAt(expiry);
            setQRCode(challenge.qr_code);
            setPhase("waiting");

            const poll = async (): Promise<void> => {
                try {
                    const result = await getWeChatChallengeStatus(challenge.transaction, controller.signal);
                    if (controller.signal.aborted) return;
                    if (result.status === "approved") {
                        const consumed = await consumeWeChatChallenge(challenge.transaction, controller.signal);
                        if (controller.signal.aborted) return;
                        if (consumed.status !== "consumed") throw new Error("Invalid WeChat challenge completion");
                        controllerRef.current = null;
                        setPhase("success");
                        timerRef.current = setTimeout(() => {
                            props.onAuthenticationStop();
                            props.onAuthenticationSuccess(consumed.redirect);
                        }, 800);
                    } else if (result.status === "expired" || result.status === "rejected") {
                        controllerRef.current = null;
                        setPhase(result.status);
                        props.onAuthenticationStop();
                    } else if (result.status === "pending" || result.status === "scanned") {
                        if (result.status === "scanned") setPhase("scanned");
                        timerRef.current = setTimeout(() => void poll(), 1800);
                    } else {
                        throw new Error("Invalid WeChat challenge status");
                    }
                } catch {
                    if (controller.signal.aborted) return;
                    controllerRef.current = null;
                    setPhase("error");
                    props.onAuthenticationStop();
                }
            };

            timerRef.current = setTimeout(() => void poll(), 1800);
        } catch {
            if (controller.signal.aborted) return;
            controllerRef.current = null;
            setPhase("error");
            props.onAuthenticationStop();
        }
    };

    startRef.current = start;

    useEffect(() => {
        if (props.disabled || autoStartedRef.current) return;

        const timer = setTimeout(() => {
            if (autoStartedRef.current) return;
            void startRef.current();
        }, 0);

        return () => clearTimeout(timer);
    }, [props.disabled]);

    useEffect(() => {
        if (phase !== "expired" || props.disabled) return;
        const timer = setTimeout(() => void startRef.current(), 0);
        return () => clearTimeout(timer);
    }, [phase, props.disabled]);

    useEffect(() => {
        if (phase !== "waiting" && phase !== "scanned") return;

        const timer = setInterval(() => {
            const remaining = Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
            if (remaining === 0) {
                controllerRef.current?.abort();
                controllerRef.current = null;
                if (timerRef.current !== null) clearTimeout(timerRef.current);
                timerRef.current = null;
                setPhase("expired");
                onAuthenticationStop();
            }
        }, 1000);

        return () => clearInterval(timer);
    }, [expiresAt, phase, onAuthenticationStop]);

    return (
        <div className="w-full">
            {phase === "idle" ? (
                <Button
                    id="wechat-sign-in-button"
                    variant="outline"
                    className="w-full"
                    disabled={props.disabled}
                    onClick={() => void start()}
                >
                    <QrCode />
                    {translate("Sign in with WeChat")}
                </Button>
            ) : (
                <div className="flex flex-col items-center gap-3 py-2 text-center" aria-live="polite">
                    {phase === "starting" || phase === "expired" ? (
                        <>
                            <Spinner size={28} />
                            <p className="text-sm">{translate("Preparing WeChat sign in")}</p>
                        </>
                    ) : null}
                    {qrCode && (phase === "waiting" || phase === "scanned") ? (
                        <div
                            className={
                                phase === "scanned"
                                    ? "hidden"
                                    : "relative h-48 w-48 overflow-hidden rounded-xl bg-white p-2"
                            }
                        >
                            <img
                                className="h-full w-full"
                                src={qrCode}
                                alt={phase === "waiting" ? translate("WeChat sign-in QR code") : ""}
                            />
                        </div>
                    ) : null}
                    {phase === "scanned" || phase === "rejected" || phase === "error" ? (
                        <>
                            <Smartphone className="h-16 w-16 text-muted-foreground" aria-hidden="true" />
                            {phase === "scanned" ? (
                                <p className="text-sm font-medium">{translate("QR code scanned")}</p>
                            ) : (
                                <div className="flex items-center justify-center gap-1">
                                    <p className="text-sm font-medium">{translate("Authorization failed")}</p>
                                    <Button
                                        variant="ghost"
                                        size="icon-xs"
                                        className="text-muted-foreground"
                                        onClick={() => void start()}
                                        disabled={props.disabled}
                                        aria-label={translate("Scan again")}
                                    >
                                        <RefreshCw aria-hidden="true" />
                                    </Button>
                                </div>
                            )}
                        </>
                    ) : null}
                    {phase === "success" ? (
                        <>
                            <CircleCheck className="h-16 w-16 text-green-600" aria-hidden="true" />
                            <p className="text-sm font-medium">{translate("WeChat sign in successful")}</p>
                        </>
                    ) : null}
                </div>
            )}
        </div>
    );
};

export default WeChatForm;
