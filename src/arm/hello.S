/* HGFW ARMv7 EABI, freestanding Linux syscall smoke test.
 * Purely print a marker and exit; no flash, network or filesystem writes.
 * Works without a C library, dynamic loader, or Broadcom binary blobs.
 */
.syntax unified
.arch armv7-a
.arm
.section .text
.global _start
.type _start, %function
_start:
    mov r0, #1             /* fd 1 = stdout */
    adr r1, message
    adr r2, message_end
    sub r2, r2, r1      /* Linux write length */
    mov r7, #4             /* ARM EABI __NR_write */
    svc #0
    mov r0, #0
    mov r7, #1             /* ARM EABI __NR_exit */
    svc #0
1:  b 1b
.align 2
message:
.ascii "HGFW ARMv7 canary: userspace ELF started successfully\n"
message_end:
.size _start, . - _start
