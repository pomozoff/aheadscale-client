import type { ManagementMode } from "@/hooks/useManagementUrl";

type Props = {
    value: ManagementMode;
    onChange: (mode: ManagementMode) => void;
    fullWidth?: boolean;
};

export const ManagementServerSwitch = (_props: Props) => (
    <span className="text-sm font-medium">Aheadscale</span>
);
