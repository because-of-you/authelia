// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { ReactNode, useEffect } from "react";

import { useTranslation } from "react-i18next";

import AppBarLoginPortal from "@components/AppBarLoginPortal";
import LoginPortalLogo from "@components/LoginPortalLogo";
import PrivacyPolicyDrawer from "@components/PrivacyPolicyDrawer";
import TypographyWithTooltip from "@components/TypographyWithTooltip";
import { EncodedName } from "@constants/constants";
import { UserInfo } from "@models/UserInfo";

export interface Props {
    id?: string;
    children?: ReactNode;
    title?: null | string;
    userInfo?: UserInfo;
}

const MinimalLayout = function (props: Props) {
    const { t: translate } = useTranslation();

    useEffect(() => {
        document.title = translate("Login - {{authelia}}", { authelia: atob(String.fromCodePoint(...EncodedName)) });
    }, [translate]);

    return (
        <div>
            <AppBarLoginPortal userInfo={props.userInfo} />
            <div id={props.id} className="flex min-h-[90vh] items-center justify-center text-center">
                <div className="mx-auto w-full max-w-[444px] px-8">
                    <div className="flex flex-col items-stretch">
                        <div className="w-full p-2">
                            <LoginPortalLogo hasWeChatAvatar={props.userInfo?.wechat_avatar} />
                        </div>
                        {props.title ? (
                            <div className="w-full">
                                <TypographyWithTooltip variant="h5" value={props.title} />
                            </div>
                        ) : null}
                        <div className="mt-2 py-2">{props.children}</div>
                    </div>
                </div>
                <PrivacyPolicyDrawer />
            </div>
        </div>
    );
};

export default MinimalLayout;
