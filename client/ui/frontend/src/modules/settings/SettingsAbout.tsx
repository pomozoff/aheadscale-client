import type { ComponentType, SVGProps } from "react";
import { useTranslation } from "react-i18next";
import { Browser } from "@wailsio/runtime";

import netbirdFull from "@/assets/logos/netbird-full.svg";
import netbirdFullLight from "@/assets/logos/netbird-full-light.svg";

// Brand glyphs from simpleicons.org (lucide deprecated its brand icons).
const GithubIcon = (props: SVGProps<SVGSVGElement>) => (
    <svg viewBox={"0 0 24 24"} fill={"currentColor"} {...props}>
        <path
            d={
                "M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"
            }
        />
    </svg>
);
import { useSettings } from "@/contexts/SettingsContext.tsx";
import { useStatus } from "@/contexts/StatusContext.tsx";
import { useAccentTrigger } from "@/modules/settings/SettingsAccent";

function openUrl(url: string) {
    Browser.OpenURL(url).catch(() => {
        window.open(url, "_blank");
    });
}

export function SettingsAbout() {
    const { t } = useTranslation();
    const { status } = useStatus();
    const { guiVersion } = useSettings();
    const daemonVersion = status?.daemonVersion ?? "—";

    const handleVersionClick = useAccentTrigger();

    const COMMUNITY_LINKS: {
        label: string;
        url: string;
        Icon: ComponentType<SVGProps<SVGSVGElement>>;
        iconClassName?: string;
    }[] = [
        {
            label: "Aheadscale · GitHub",
            url: "https://github.com/pomozoff/aheadscale-client",
            Icon: GithubIcon,
            iconClassName: "h-3 w-3",
        },
        {
            label: "NetBird · BSD license",
            url: "https://github.com/netbirdio/netbird/blob/v0.80.0/LICENSE",
            Icon: GithubIcon,
            iconClassName: "h-3 w-3",
        },
    ];

    return (
        <div
            className={
                "mx-auto flex min-h-[calc(100vh-12rem)] max-w-2xl flex-col items-center justify-center gap-4"
            }
        >
            <img
                src={netbirdFullLight}
                alt={t("common.netbird")}
                className={"h-7 w-auto dark:hidden"}
            />
            <img
                src={netbirdFull}
                alt={t("common.netbird")}
                className={"hidden h-7 w-auto dark:block"}
            />
            <div className={"flex flex-col items-center gap-0.5 text-center"}>
                <button
                    type={"button"}
                    onClick={handleVersionClick}
                    className={
                        "cursor-text select-text bg-transparent text-sm font-semibold text-nb-gray-100 outline-none"
                    }
                >
                    {daemonVersion === "development" ? (
                        <span>
                            {t("settings.about.clientName")}{" "}
                            <span className={"font-mono text-yellow-400"}>
                                {t("settings.about.development")}
                            </span>
                        </span>
                    ) : (
                        t("settings.about.client", { version: daemonVersion })
                    )}
                </button>
                <p className={"cursor-text select-text text-sm font-medium text-nb-gray-250"}>
                    {guiVersion === "development" ? (
                        <span>
                            {t("settings.about.guiName")}{" "}
                            <span className={"font-mono text-yellow-400"}>
                                {t("settings.about.development")}
                            </span>
                        </span>
                    ) : (
                        t("settings.about.gui", { version: guiVersion })
                    )}
                </p>
            </div>

            <p className={"mt-2 text-center text-sm text-nb-gray-300"}>
                {t("settings.about.copyright", { year: new Date().getFullYear() })}
            </p>
            <div
                className={"flex flex-wrap justify-center gap-x-4 gap-y-1 text-xs text-nb-gray-200"}
            >
                {COMMUNITY_LINKS.map(({ label, url, Icon, iconClassName }) => (
                    <button
                        key={url}
                        type={"button"}
                        tabIndex={0}
                        onClick={() => openUrl(url)}
                        className={
                            "inline-flex items-center gap-1.5 rounded-sm decoration-[0.5px] underline-offset-4 outline-none transition hover:text-nb-gray-100 hover:underline focus-visible:ring-2 focus-visible:ring-nb-gray-50/60 focus-visible:ring-offset-2 focus-visible:ring-offset-nb-gray-940"
                        }
                    >
                        <Icon aria-hidden={"true"} className={iconClassName ?? "h-3.5 w-3.5"} />
                        <span>{label}</span>
                    </button>
                ))}
            </div>
        </div>
    );
}
