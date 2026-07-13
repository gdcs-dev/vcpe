cmake_minimum_required(VERSION 3.10)
project(apparmor-simulator C)

set(CMAKE_C_STANDARD 11)

add_executable(apparmor-simulator
    apparmor-simulator.c
    events.c
)

target_include_directories(apparmor-simulator PRIVATE
    ${CMAKE_INSTALL_PREFIX}/include
    ${CMAKE_INSTALL_PREFIX}/include/wrp-c
    ${CMAKE_INSTALL_PREFIX}/include/nopoll
    ${CMAKE_INSTALL_PREFIX}/include/rbus
    ${CMAKE_INSTALL_PREFIX}/include/rtmessage
    /usr/include/cjson
)

target_link_directories(apparmor-simulator PRIVATE
    ${CMAKE_INSTALL_PREFIX}/lib
)

# Link order: wrp-c -> libparodus (nanomsg) -> rbus -> cjson -> pthread
# cimplog is a transitive dependency of libwrp-c (provides __cimplog).
target_link_libraries(apparmor-simulator
    wrp-c
    libparodus
    nanomsg
    rbus
    cjson
    cimplog
    pthread
)

install(TARGETS apparmor-simulator
    RUNTIME DESTINATION bin
)
