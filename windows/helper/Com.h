// A small owner of a COM interface pointer (no ATL needed).
#pragma once

#include <unknwn.h>


namespace helper {


template <class T>
class Com
{
 public:
    Com() = default;
    Com(const Com&) = delete;
    Com& operator=(const Com&) = delete;
    ~Com() { reset(); }

    T*  get() const { return m_ptr; }
    T*  operator->() const { return m_ptr; }
    explicit operator bool() const { return m_ptr != nullptr; }

    /// For an out parameter: the old interface is released, the new one is taken.
    T** operator&() { reset(); return &m_ptr; }

    void reset()
    {
        if (m_ptr)
        {
            m_ptr->Release();
            m_ptr = nullptr;
        }
    }

 private:
    T*  m_ptr = nullptr;
};


} //namespace helper
