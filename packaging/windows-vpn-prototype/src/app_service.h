#pragma once
#include <winrt/Windows.ApplicationModel.Background.h>
namespace aheadscale {
bool TryRunAppService(winrt::Windows::ApplicationModel::Background::IBackgroundTaskInstance const& task);
int ProbeAppServiceCaller();
}
