#pragma once
#include <winrt/Windows.ApplicationModel.Background.h>
#include <winrt/Windows.ApplicationModel.AppService.h>
namespace aheadscale {
bool TryRunAppService(winrt::Windows::ApplicationModel::Background::IBackgroundTaskInstance const& task);
int ProbeAppServiceCaller();
winrt::Windows::ApplicationModel::AppService::AppServiceConnection ActiveBroker(bool nativeOnly=false);
}
