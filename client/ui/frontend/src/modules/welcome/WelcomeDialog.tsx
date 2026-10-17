import { useState } from "react";
import { Preferences, WindowManager } from "@bindings/services";
import { useTranslation } from "react-i18next";
import { ConfirmDialog } from "@/components/dialog/ConfirmDialog";
import { DialogHeading } from "@/components/dialog/DialogHeading";
import { DialogDescription } from "@/components/dialog/DialogDescription";
import { DialogActions } from "@/components/dialog/DialogActions";
import { Button } from "@/components/buttons/Button";
import { useAutoSizeWindow } from "@/hooks/useAutoSizeWindow";
import { startConnection } from "@/lib/connection";

export default function WelcomeDialog() {
    const { t } = useTranslation();
    const [busy, setBusy] = useState(false);
    const contentRef = useAutoSizeWindow<HTMLDivElement>(360, true);

    const connect = async () => {
        if (busy) return;
        setBusy(true);
        try {
            // The daemon completes SSO and connects even when the webview is hidden.
            await Preferences.SetOnboardingCompleted(true);
            await WindowManager.OpenMain();
            await startConnection();
            await WindowManager.CloseWelcome();
        } finally {
            setBusy(false);
        }
    };

    return (
        <ConfirmDialog ref={contentRef} aria-labelledby="aheadscale-welcome-title">
            <DialogHeading id="aheadscale-welcome-title" align="left">
                {t("welcome.title")}
            </DialogHeading>
            <DialogDescription align="left">{t("welcome.description")}</DialogDescription>
            <DialogActions>
                <Button
                    autoFocus
                    variant="primary"
                    className="w-full"
                    loading={busy}
                    onClick={connect}
                >
                    {t("welcome.continue")}
                </Button>
            </DialogActions>
        </ConfirmDialog>
    );
}
