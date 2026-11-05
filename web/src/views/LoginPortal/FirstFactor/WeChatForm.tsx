// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";

import { QrCode } from "lucide-react";
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

type Phase = "error" | "expired" | "idle" | "rejected" | "starting" | "waiting";

const WeChatForm = function (props: Props) {
    const { t: translate } = useTranslation();
    const targetURL = useQueryParam(RedirectionURL);
    const requestMethod = useQueryParam(RequestMethod);
    const { flow, id: flowID, subflow } = useFlow();
    const userCode = useUserCode();
    const [phase, setPhase] = useState<Phase>("idle");
    const [qrCode, setQRCode] = useState("");
    const controllerRef = useRef<AbortController | null>(null);
    const timerRef = useRef<null | ReturnType<typeof setTimeout>>(null);

    const clearAttempt = () => {
        controllerRef.current?.abort();
        controllerRef.current = null;
        if (timerRef.current !== null) {
            clearTimeout(timerRef.current);
            timerRef.current = null;
        }
    };

    useEffect(() => () => clearAttempt(), []);

    const close = () => {
        clearAttempt();
        setQRCode("");
        setPhase("idle");
        props.onAuthenticationStop();
    };

    const start = async () => {
        if (phase === "starting" || phase === "waiting") return;
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
            if (!challenge.transaction || !isSafeWeChatQRCode(challenge.qr_code)) {
                throw new Error("Invalid WeChat challenge");
            }
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
                        props.onAuthenticationStop();
                        props.onAuthenticationSuccess(consumed.redirect);
                    } else if (result.status === "expired" || result.status === "rejected") {
                        controllerRef.current = null;
                        setPhase(result.status);
                        props.onAuthenticationStop();
                    } else if (result.status === "pending") {
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
                <div
                    className="flex flex-col items-center gap-3 rounded-lg border border-border p-4 text-center"
                    aria-live="polite"
                >
                    {phase === "starting" ? (
                        <>
                            <Spinner size={28} />
                            <p className="text-sm">{translate("Preparing WeChat sign in")}</p>
                        </>
                    ) : null}
                    {phase === "waiting" ? (
                        <>
                            <img
                                className="h-48 w-48 rounded bg-white p-2"
                                src={qrCode}
                                alt={translate("WeChat sign-in QR code")}
                            />
                            <p className="text-sm font-medium">{translate("Scan with WeChat and confirm sign in")}</p>
                            <p className="text-xs text-muted-foreground">{translate("Waiting for confirmation")}</p>
                        </>
                    ) : null}
                    {phase === "expired" ? (
                        <p className="text-sm">{translate("WeChat sign-in request expired")}</p>
                    ) : null}
                    {phase === "rejected" ? (
                        <p className="text-sm">{translate("WeChat sign-in request was declined")}</p>
                    ) : null}
                    {phase === "error" ? (
                        <p className="text-sm">{translate("Unable to complete WeChat sign in")}</p>
                    ) : null}
                    {phase === "expired" || phase === "rejected" || phase === "error" ? (
                        <Button variant="outline" onClick={() => void start()} disabled={props.disabled}>
                            {translate("Try again")}
                        </Button>
                    ) : null}
                    <Button variant="ghost" onClick={close}>
                        {translate("Cancel")}
                    </Button>
                </div>
            )}
        </div>
    );
};

export default WeChatForm;
