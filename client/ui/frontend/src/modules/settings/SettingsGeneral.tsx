import { useTranslation } from "react-i18next";
import FancyToggleSwitch from "@/components/switches/FancyToggleSwitch";
import { SectionGroup } from "@/modules/settings/SettingsSection.tsx";
import { useAutostartSetting, useSettings } from "@/contexts/SettingsContext.tsx";
import { LanguagePicker } from "@/components/LanguagePicker.tsx";
import { ThemePicker } from "@/components/ThemePicker.tsx";
import { useRestrictions } from "@/contexts/RestrictionsContext.tsx";
import { useKeepConnectedOnQuit } from "@/hooks/useKeepConnectedOnQuit.ts";

export function SettingsGeneral() {
    const { t } = useTranslation();
    const { config, setField } = useSettings();
    const { autostart, setAutostartEnabled } = useAutostartSetting();
    const { mdm, features } = useRestrictions();
    const { keepConnected, setKeepConnectedOnQuit } = useKeepConnectedOnQuit();

    return (
        <>
            <SectionGroup title={t("settings.general.section.general")}>
                <LanguagePicker />
                <ThemePicker />
                <FancyToggleSwitch
                    value={!config.disableNotifications}
                    onChange={(v) => setField("disableNotifications", !v)}
                    label={t("settings.general.notifications.label")}
                    helpText={t("settings.general.notifications.help")}
                />
                {!mdm.disableAutoConnect && !features.disableUpdateSettings && (
                    <FancyToggleSwitch
                        value={!config.disableAutoConnect}
                        onChange={(v) => setField("disableAutoConnect", !v)}
                        label={t("settings.general.connectOnStartup.label")}
                        helpText={t("settings.general.connectOnStartup.help")}
                    />
                )}
                {(autostart === null || autostart.supported) && (
                    <FancyToggleSwitch
                        value={autostart?.enabled ?? false}
                        onChange={setAutostartEnabled}
                        loading={autostart === null}
                        label={t("settings.general.autostart.label")}
                        helpText={t("settings.general.autostart.help")}
                    />
                )}
                <FancyToggleSwitch
                    value={keepConnected ?? false}
                    onChange={(v) => {
                        void setKeepConnectedOnQuit(v);
                    }}
                    loading={keepConnected === null}
                    label={t("settings.general.keepConnectedOnQuit.label")}
                    helpText={t("settings.general.keepConnectedOnQuit.help")}
                />
            </SectionGroup>
        </>
    );
}
