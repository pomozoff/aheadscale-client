#include "native_channel.h"
#include <windows.h>
int WINAPI wWinMain(HINSTANCE,HINSTANCE,PWSTR,int){
 winrt::init_apartment(winrt::apartment_type::multi_threaded);
 try{return aheadscale::RunNativeBroker();}catch(...){return 1;}
}
