// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";

import UserSvg from "@assets/images/user.svg?react";
import { getBasePath } from "@utils/BasePath";
import { getLogoOverride } from "@utils/Configuration";

export default function LoginPortalLogo({ hasWeChatAvatar }: { hasWeChatAvatar?: boolean }) {
    const [avatarFailed, setAvatarFailed] = useState(false);

    if (hasWeChatAvatar && !avatarFailed) {
        return (
            <img
                src={`${getBasePath()}/api/user/wechat-avatar`}
                alt=""
                className="mx-auto my-2 h-16 w-16 rounded-full object-cover"
                onError={() => setAvatarFailed(true)}
            />
        );
    }

    if (getLogoOverride()) {
        return (
            <img
                src="./static/media/logo.png"
                alt="Logo"
                className="mx-auto my-2 w-16"
                style={{ fill: "var(--custom-icon)" }}
            />
        );
    }

    return <UserSvg className="mx-auto my-2 w-16" style={{ fill: "var(--custom-icon)" }} />;
}
